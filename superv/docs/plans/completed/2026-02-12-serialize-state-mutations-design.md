# Serialize State-Mutating OpAMP Callbacks

## Problem

OpAMP callbacks fire on opamp-go library goroutines with no ordering guarantees. The current
code has race conditions:

1. **Health goroutine** accesses `s.opampClient` without any lock (line ~288), risking nil
   dereference during `reconnectClient`.
2. **`OnOpampConnectionSettings`** spawns an untracked goroutine (`go func()`) that races with
   other callbacks and always returns `nil`, losing error semantics that opamp-go needs to
   report `ConnectionSettingsStatus` upstream.
3. **`reconnectClient`** has a lock-release window between stopping the old client and assigning
   the new one where concurrent readers can observe inconsistent state.

## Design

Split callbacks into two categories:

- **Direct callbacks** (no queue): `OnConnect`, `OnConnectFailed`, `OnError`, `OnCustomMessage`,
  `OnRemoteConfig`. These do not mutate connection/client lifecycle state and have no ordering
  dependencies with each other or the worker.
- **Serialized via worker**: State-mutating operations that involve stopping/starting the
  opamp-go client (and future `OnPackagesAvailable`, `OnCommand`). These must not run
  concurrently.

### Worker Fields

```go
type Supervisor struct {
    // ... existing fields ...

    // Serialized worker for state-mutating operations.
    workQueue  chan workItem
    workCtx    context.Context
    workCancel context.CancelFunc
    workWg     sync.WaitGroup
}

type workItem struct {
    fn func(ctx context.Context)
}
```

Note: `workItem` uses `func(ctx context.Context)` (no error return) because the worker only
handles fire-and-forget operations. See [Two-Phase Connection Settings](#two-phase-connection-settings)
for why enqueue-and-wait is not viable for `OnOpampConnectionSettings`.

### Worker Loop

Started in `Start()` **before** `createAndStartClient()`, stopped in `Stop()`. The worker must
be running before the OpAMP client starts, otherwise early callbacks can block on the
unbuffered channel with no consumer.

Startup order in `Start()`:

1. Initialize `workQueue`, `workCtx`, `workCancel`.
2. `s.workWg.Add(1)` then `go s.runWorker()`.
3. Call `createAndStartClient()` (which starts the OpAMP client and may trigger callbacks).

Shutdown order in `Stop()`:

1. Cancel `workCtx`.
2. Stop OpAMP client (stops invoking callbacks).
3. `workWg.Wait()` (worker exits via `<-s.workCtx.Done()`).

```go
func (s *Supervisor) runWorker() {
    defer s.workWg.Done()
    for {
        select {
        case item := <-s.workQueue:
            item.fn(s.workCtx)
        case <-s.workCtx.Done():
            return
        }
    }
}
```

The worker passes `workCtx` (derived from the supervisor lifecycle context) so all in-flight
work is cancelled on shutdown. Individual operations can derive tighter timeouts from `workCtx`.

**Note:** When both `s.workQueue` and `s.workCtx.Done()` are ready simultaneously, Go's
`select` picks randomly. A work item can start executing with an already-cancelled context.
This is safe as long as work functions check `ctx.Err()` early (e.g. `reconnectClient` will
fail fast on a cancelled context when calling `client.Stop()` or `createAndStartClient`).

### Enqueue Helper

```go
func (s *Supervisor) enqueueWork(ctx context.Context, fn func(ctx context.Context)) bool {
    select {
    case s.workQueue <- workItem{fn: fn}:
        return true
    case <-ctx.Done():
        return false
    }
}
```

Fire-and-forget: sends work to the queue, or gives up if the caller's context is cancelled.
Returns whether the work was successfully enqueued so the caller can clean up on failure.

### Channel: Unbuffered

`workQueue` is `make(chan workItem)` (unbuffered). This provides natural back-pressure: if the
worker is busy with a reconnect, a second operation blocks at the enqueue stage rather than
queuing up silently.

### reconnectClient Refactor

The current `reconnectClient` has two problems:

1. **Stopped-client window:** Between stopping the old client (line 571) and assigning the new
   one (line 583), `s.opampClient` points to a stopped client. Concurrent readers (health
   goroutine, `OnRemoteConfig`) can snapshot and call methods on the stopped client.

2. **Double-stop of opamp-go client:** opamp-go's `ClientCommon.Stop()` does not guard against
   double-stop — `isStarted` is set true during `StartConnectAndRun` (clientcommon.go:240) and
   never reset. A second `Stop()` calls `cancelFunc()` (no-op) then blocks on
   `<-c.stoppedSignal` which was already consumed by the first `Stop()`. This blocks until the
   context timeout. This affects rollback in `applyConnectionSettings` (if
   `createAndStartClient` fails after stopping the old client) and the `Stop()` vs worker race.

**Fix:** Nil `s.opampClient` under lock immediately before stopping the old client. This
ensures concurrent readers (health goroutine, `OnRemoteConfig`) see nil rather than a stopped
client. Check `s.running` before assigning the new client to prevent orphaned clients during
shutdown.

If the current client is already nil (e.g. a previous reconnect failed after nilling but before
assigning a new client), skip the stop step and go straight to creating a new client. This is
essential for rollback in `applyConnectionSettings`: after a failed first reconnect leaves
`s.opampClient = nil`, rollback must still be able to reconnect with old settings. Note that
`createAndStartClient` can also fail in this nil-client rollback scenario, leaving the
supervisor with no OpAMP client until the next connection settings update or restart.

```go
func (s *Supervisor) reconnectClient(ctx context.Context, settings connection.Settings) error {
    s.mu.Lock()
    client := s.opampClient
    s.opampClient = nil  // Nil immediately so concurrent readers see nil, not stopped client.
    s.mu.Unlock()

    // If client is nil, skip the stop step. This happens during rollback when a previous
    // reconnect failed after nilling s.opampClient but before assigning a new client.
    if client != nil {
        s.logger.Info("Stopping OpAMP client for connection settings update")
        if err := client.Stop(ctx); err != nil {
            return fmt.Errorf("failed to stop client: %w", err)
        }
    }

    s.logger.Info("Starting OpAMP client with new connection settings",
        zap.String("endpoint", settings.Endpoint))
    newClient, err := s.createAndStartClient(ctx, settings)
    if err != nil {
        return fmt.Errorf("apply connection settings: %w", err)
    }

    // Check running state before assigning. If Stop() has been called concurrently,
    // discard the new client to prevent an orphaned client with a running goroutine.
    s.mu.Lock()
    if !s.running {
        s.mu.Unlock()
        // Best-effort stop: workCtx is already cancelled (Stop() calls workCancel()
        // before client.Stop()), so opamp-go's Stop() returns ctx.Err() immediately.
        // The client's internal goroutine self-cleans via its cancelled runCtx.
        _ = newClient.Stop(ctx)
        return fmt.Errorf("supervisor stopped during reconnect")
    }
    s.opampClient = newClient
    s.mu.Unlock()

    return nil
}
```

This eliminates the stopped-client window, the orphaned-client race with `Stop()`, and
supports rollback when the current client is nil.

### Lifecycle Contract

**`Start()` and `Stop()` are NOT safe for concurrent use.** The caller must ensure:

- `Start()` completes before `Stop()` is called.
- `Stop()` completes before a subsequent `Start()` is called.
- At most one goroutine calls `Start()` or `Stop()` at a time.

This matches the current code (supervisor.go): `Start()` does not hold `mu` for its entire
duration and writes fields like `healthCancel` and `commander` without synchronization. This is
safe only because the caller guarantees sequential Start/Stop lifecycle. The `running` flag
under `mu` guards against double-start and double-stop, but not against concurrent
Start-and-Stop.

### Shutdown

**`Stop()` must release `s.mu` before calling `opampClient.Stop()`.** The current `Stop()`
holds `s.mu.Lock()` for the entire function (supervisor.go:309) and calls
`opampClient.Stop()` at line 333 while still holding it. opamp-go's `Stop()` waits for
in-flight callbacks to return, but callbacks can acquire `s.mu` (e.g.
`handleEnrollmentCertificate` at line 718). This is a deadlock: `Stop()` holds `mu` and waits
for the callback, the callback waits for `mu` held by `Stop()`.

This is a pre-existing bug, not introduced by this design, but must be fixed as part of this
work.

**Fix:** Refactor `Stop()` to snapshot and nil-out the fields it needs under `mu`, then
release `mu` before calling `Stop()` on the snapshotted references. Since Start()/Stop() are
never concurrent (see [Lifecycle Contract](#lifecycle-contract)), fields like `healthCancel`
and `commander` can be read safely after `mu` is released — no concurrent writer exists.

```go
func (s *Supervisor) Stop(ctx context.Context) error {
    s.mu.Lock()
    if !s.running {
        s.mu.Unlock()
        return nil
    }
    s.running = false

    // Snapshot and nil-out to prevent concurrent use after unlock.
    client := s.opampClient
    server := s.opampServer
    s.opampClient = nil
    s.opampServer = nil
    s.mu.Unlock()

    // Cancel worker context first (signals in-flight work to abort).
    s.workCancel()

    // Fields below (healthCancel, commander) are safe to read without mu because
    // Start() and Stop() are never concurrent (see Lifecycle Contract).
    if s.healthCancel != nil {
        s.healthCancel()
    }
    s.healthWg.Wait()

    if s.commander != nil {
        if err := s.commander.Stop(ctx); err != nil {
            s.logger.Error("Error stopping agent", zap.Error(err))
        }
    }

    if client != nil {
        if err := client.Stop(ctx); err != nil {
            s.logger.Error("Error stopping OpAMP client", zap.Error(err))
        }
    }

    if server != nil {
        if err := server.Stop(ctx); err != nil {
            s.logger.Error("Error stopping OpAMP server", zap.Error(err))
        }
    }

    s.workWg.Wait()

    return nil
}
```

Shutdown order in `Stop()` (must match the order in [Worker Loop](#worker-loop)):

1. Cancel `workCtx` (signals in-flight work to abort).
2. Stop health goroutine and commander. During this window the OpAMP client is still running,
   so callbacks can still fire. Since `workCtx` is cancelled, the worker may have already
   exited, so `enqueueWork` callers block on the unbuffered channel until step 3 cancels
   their callback context. Callback latency during this window is bounded by commander
   shutdown time.
3. Stop OpAMP client — `client.Stop()` cancels callback contexts and waits for in-flight
   callbacks to return. After this, no new callbacks are invoked, so no new `enqueueWork` calls
   occur. **Must not hold `s.mu` during this call.**
4. `workWg.Wait()` — the worker exits via `<-s.workCtx.Done()`. Safe because step 3 guarantees
   no producers remain.

**The channel is never closed.** Closing a channel with concurrent producers causes a panic
(send on closed channel). Since opamp-go callbacks are the producers and run on library
goroutines, there is no safe point where we can guarantee all producers have stopped before
closing. Shutdown relies on context cancellation instead.

How each goroutine unblocks:
- **Worker goroutine:** `<-s.workCtx.Done()` branch in select loop (step 1).
- **enqueueWork callers:** `<-ctx.Done()` branch in select (callback context cancelled by
  step 3).
- **In-flight work item:** `workCtx` is cancelled (step 1), work function observes cancellation
  and returns.

**`Stop()` must be called with a bounded context timeout.** If a callback is blocked on disk
I/O (phase 1 writes in `prepareConnectionSettings`), opamp-go's `client.Stop()` waits for it
to return. A context timeout ensures the caller's shutdown path completes even with a hung
filesystem.

**Caveat:** A timed-out `Stop()` is best-effort. opamp-go's `Stop()` returns `ctx.Err()`
before `stoppedSignal` fires (clientcommon.go:210-213), meaning callbacks and the receiver
goroutine may still be running. The "fully stopped" guarantee only holds when `Stop()` returns
nil. Callers that time out should assume background work may still be in flight and proceed
with process exit.

### Two-Phase Connection Settings

The `OnOpampConnectionSettings` callback cannot use enqueue-and-wait because of a circular
dependency with opamp-go's `client.Stop()`:

1. The callback blocks waiting for the worker result.
2. The worker calls `reconnectClient` → `client.Stop()`.
3. opamp-go's `Stop()` cancels in-flight callback contexts and **waits for them to return**.
4. Deadlock: callback waits for worker, worker (via `Stop`) waits for callback.

Context cancellation breaks the deadlock, but the callback returns `ctx.Err()` while the
reconnect may still succeed — causing a false FAILED status.

**Solution: split into two phases.**

#### Phase 1: `prepareConnectionSettings` (synchronous in callback)

Runs directly in the `OnOpampConnectionSettings` callback. Performs all local, fast operations
that can produce a meaningful success/failure status:

1. Nil settings check.
2. `handleEnrollmentCertificate` — local crypto operations, can fail. Writes credential files
   to disk (`auth/manager.go:CompleteEnrollment`).
3. Heartbeat interval update — updates the supervisor wrapper's stored interval value for use
   when creating a new client after reconnect. Note: opamp-go already updates the sender's
   heartbeat interval in `rcvOpampConnectionSettings` (receivedprocessor.go:266-271) *before*
   invoking the callback, so this step only affects the wrapper's state, not the current
   sender.
4. `connectionSettingsManager.SettingsChanged` — pure comparison, determines if reconnect is
   needed.
5. `connectionSettingsManager.StageNext` — writes new settings to a temp file on disk, can fail.

**Note on disk I/O in phase 1:** Steps 2 and 5 perform local disk writes. This is intentional
— they are in phase 1 specifically for accurate callback error reporting, which is the only
reliable failure signaling path given current opamp-go limitations.

- **Risk:** A hung filesystem blocks the callback, which blocks opamp-go's receiver loop and
  ultimately `Stop()`. To bound shutdown time, `Stop()` must always be called with a context
  timeout (see [Shutdown](#shutdown)).
- **Future:** Once opamp-go exposes a public `SetConnectionSettingsStatus` API, these writes
  can move to phase 2 where late status reporting eliminates the need for synchronous error
  feedback.

Returns `(*pendingReconnect, error)`. If error is non-nil, the callback returns it to opamp-go.
If `pendingReconnect` is nil, no reconnect is needed (return nil).

```go
type pendingReconnect struct {
    newSettings connection.Settings
    oldSettings connection.Settings
    stagedFile  persistence.StagedFile
}

func (s *Supervisor) prepareConnectionSettings(
    ctx context.Context,
    settings *protobufs.OpAMPConnectionSettings,
) (*pendingReconnect, error) {
    if settings == nil {
        return nil, nil
    }

    newlyEnrolled, err := s.handleEnrollmentCertificate(settings)
    if err != nil {
        return nil, fmt.Errorf("enrollment certificate handling failed: %w", err)
    }

    // Heartbeat interval update (no reconnect needed).
    // Only updates the wrapper's stored value for future reconnects —
    // opamp-go already updated the sender's interval before this callback.
    if interval := settings.GetHeartbeatIntervalSeconds(); interval > 0 {
        // ... update heartbeat on current client ...
    }

    newSettings, changed := s.connectionSettingsManager.SettingsChanged(settings)
    if !changed && !newlyEnrolled {
        return nil, nil
    }

    stagedFile, err := s.connectionSettingsManager.StageNext(newSettings)
    if err != nil {
        return nil, fmt.Errorf("failed to persist new settings: %w", err)
    }

    oldSettings := s.connectionSettingsManager.GetCurrent()
    return &pendingReconnect{
        newSettings: newSettings,
        oldSettings: oldSettings,
        stagedFile:  stagedFile,
    }, nil
}
```

#### Phase 2: `applyConnectionSettings` (fire-and-forget on worker)

Enqueued onto the worker only if phase 1 returned a non-nil `pendingReconnect`. Handles the
slow, destructive reconnection:

1. `reconnectClient(newSettings)` — stops old client, creates and starts new one.
2. On failure: `stagedFile.Cleanup()`, rollback via `reconnectClient(oldSettings)`.
3. `stagedFile.Commit()` — atomically replaces persisted settings file.
4. On commit failure: rollback/recovery logic (unchanged from current code).

```go
func (s *Supervisor) applyConnectionSettings(ctx context.Context, pending *pendingReconnect) {
    if err := s.reconnectClient(ctx, pending.newSettings); err != nil {
        s.logger.Error("Failed to connect with new settings, rolling back", zap.Error(err))
        if cleanupErr := pending.stagedFile.Cleanup(); cleanupErr != nil {
            s.logger.Error("Failed to clean up staged settings file", zap.Error(cleanupErr))
        }
        if rollbackErr := s.reconnectClient(ctx, pending.oldSettings); rollbackErr != nil {
            s.logger.Error("Rollback also failed", zap.Error(rollbackErr))
        }
        return
    }

    // Commit atomically replaces the persisted settings file. The commit callback
    // wired by StageNext (settings.go:268-270) calls SetCurrent(newSettings), updating
    // the in-memory baseline to match the now-running client.
    if err := pending.stagedFile.Commit(); err != nil {
        s.logger.Error("Failed to commit staged settings file", zap.Error(err))
        // ... rollback/recovery logic (unchanged from current code). Note: the commit
        // callback did NOT run (Commit failed), so the in-memory baseline still holds
        // oldSettings. The recovery path's direct SetCurrent calls remain correct. ...
        return
    }

    s.logger.Info("Connection settings applied successfully")
}
```

#### Callback Wiring

```go
OnOpampConnectionSettings: func(ctx context.Context, settings *protobufs.OpAMPConnectionSettings) error {
    s.logger.Info("Received connection settings update")

    // Phase 1: validate and prepare (synchronous, returns status to opamp-go)
    pending, err := s.prepareConnectionSettings(ctx, settings)
    if err != nil {
        return err
    }
    if pending == nil {
        return nil // no reconnection needed
    }

    // Phase 2: reconnect (async on worker, can't block callback)
    if !s.enqueueWork(ctx, func(wCtx context.Context) {
        s.applyConnectionSettings(wCtx, pending)
    }) {
        // Enqueue failed (context cancelled). This can happen during shutdown
        // (client.Stop() cancels callback contexts) or if opamp-go cancels the callback
        // context because processing takes too long (callback contract, callbacks.go:104).
        //
        // Commit the staged file so the new settings are persisted to disk. They will be
        // applied on next supervisor restart. We cannot clean up and rely on the server
        // re-sending — the OpAMP spec says the server SHOULD NOT re-send connection
        // settings if the destination is unchanged (spec:2370-2372).
        //
        // This creates a temporary in-memory/on-disk mismatch (current client uses old
        // settings, disk has new settings), which self-heals on restart.
        //
        // Return nil rather than error — returning error would store FAILED for this
        // settings hash in opamp-go's in-process state, and
        // updateStoredConnectionSettingsStatus does not allow a same-hash FAILED → APPLIED
        // transition, creating sticky poison. (Note: this sticky FAILED is scoped to the
        // current client instance's ClientSyncedState — a reconnect creates a fresh
        // opamp-go client with empty state, so it would not carry over. But in this path
        // we do NOT reconnect, so the old client continues and the poison persists.)
        s.logger.Warn("Failed to enqueue connection settings apply (context cancelled), persisting for next restart")
        if commitErr := pending.stagedFile.Commit(); commitErr != nil {
            // Both apply (enqueue) and persist (commit) failed — settings are truly lost.
            // Return error to report FAILED. The sticky FAILED concern does not apply here
            // because there is no future success for this hash to block — we failed
            // completely. FAILED gives the server accurate information rather than
            // misleading with false APPLIED (the server SHOULD NOT resend unchanged
            // settings per spec, so false APPLIED can permanently lose the update).
            s.logger.Error("Failed to commit staged settings file", zap.Error(commitErr))
            // Cleanup() is safe to call after Commit() failure — both renameio and
            // winrenameio implementations are idempotent (done flag prevents double-remove).
            if cleanupErr := pending.stagedFile.Cleanup(); cleanupErr != nil {
                s.logger.Error("Failed to clean up staged settings file", zap.Error(cleanupErr))
            }
            return fmt.Errorf("failed to persist settings for deferred apply: %w", commitErr)
        }

        // Commit succeeded, but StageNext's commit callback (settings.go:268-270)
        // has already called SetCurrent(newSettings), changing the in-memory baseline.
        // Restore old settings so that SettingsChanged() and rollback decisions in
        // subsequent callbacks remain correct for the current runtime. The new settings
        // are persisted on disk and will take effect on next restart.
        //
        // Note: There is a brief window between the commit callback's
        // SetCurrent(newSettings) and the restore below where a concurrent
        // SettingsChanged() call could see newSettings as the baseline. This is
        // acceptable: the enqueue failure path only runs during context cancellation
        // (shutdown or opamp-go timeout), so no subsequent OnOpampConnectionSettings
        // callback can arrive — the client is either stopping or has already cancelled
        // further processing. No lock is needed here.
        s.connectionSettingsManager.SetCurrent(pending.oldSettings)
    }

    // TODO: Returning nil here reports APPLIED to the server, but reconnect has not completed
    // yet. This is optimistic — see "Status Reporting Limitations" below.
    return nil
},
```

#### ReportsConnectionSettingsStatus Capability

For opamp-go to send `ConnectionSettingsStatus` upstream based on callback return values,
the agent must advertise the `ReportsConnectionSettingsStatus` capability. Currently it does
not: `opamp.Capabilities` has no such field, and `ToProto()` does not set the bit
(client.go:65-128).

Without this capability, opamp-go's `rcvOpampConnectionSettings`
(receivedprocessor.go:261, capability check at line 278) skips the entire status block — the
callback error is only logged, never sent upstream.

**Required changes:**

1. Add `ReportsConnectionSettingsStatus bool` to `opamp.Capabilities`.
2. Add the corresponding `ToProto()` mapping for
   `AgentCapabilities_AgentCapabilities_ReportsConnectionSettingsStatus`.
3. Set `ReportsConnectionSettingsStatus: true` in `createAndStartClient` alongside
   `AcceptsOpAMPConnectionSettings: true`.

This enables the callback error path to drive status reporting end-to-end, with the
optimistic APPLIED semantics documented below.

#### Status Reporting Limitations

The OpAMP protobuf defines `ConnectionSettingsStatuses_APPLIED` as "successfully applied by
the Agent" (opamp.pb.go, `ConnectionSettingsStatuses` enum). With the two-phase design,
returning nil from the callback before reconnect completes is **optimistic**: it reports
APPLIED when the settings have only been validated and staged, not yet applied.

This is the least-bad option given the constraints:

- **Enqueue-and-wait** is impossible (circular dependency with `client.Stop()`, see above).
- **Always returning error** would report FAILED for every reconnect, including successful ones.
- **Optimistic APPLIED** is correct in the common case (most reconnects succeed). When
  reconnect fails, the rollback logic self-heals, and the next server interaction uses the
  original settings. When enqueue fails, settings are persisted to disk and applied on
  next restart (the server will not re-send unchanged settings per OpAMP spec).

The correct fix is sending a **late corrected status** after reconnect completes. This requires
opamp-go to expose a public `SetConnectionSettingsStatus` API, which it currently does not
(client.go:395 — the wrapper is a no-op that logs). When opamp-go adds this, the worker
should send FAILED/APPLIED after `applyConnectionSettings` finishes.

`reportConnectionSettingsStatus` becomes dead code and is deleted. `SetConnectionSettingsStatus`
(`opamp/client.go`) is kept as a wrapper with a TODO for future use.

**Code TODOs to add during implementation:**

1. In `OnOpampConnectionSettings` callback wiring: note that the return value only reflects
   accept/stage, not the final reconnect outcome.
2. In `SetConnectionSettingsStatus` wrapper (`opamp/client.go`): note that once opamp-go
   exposes a public status update API, `applyConnectionSettings` should call this method to
   send late FAILED/APPLIED after async reconnect completes.

### Health Goroutine Race Fix

Apply the RLock snapshot pattern (same as `OnRemoteConfig` and `forwardCustomMessage`):

```go
s.healthWg.Go(func() {
    for status := range healthUpdates {
        if s.authManager.IsEnrolled() {
            s.mu.RLock()
            client := s.opampClient
            s.mu.RUnlock()

            if client != nil {
                if err := client.SetHealth(status.ToComponentHealth(nil)); err != nil {
                    s.logger.Warn("Failed to report health", zap.Error(err))
                }
            }
        }
    }
})
```

Not routed through the work queue: health is high-frequency, read-only from supervisor's
perspective, and shouldn't compete with connection settings for queue position.

### Mutex Audit

With the `Stop()` refactor (snapshot-and-nil, release `mu` before stopping components):

| Field | Still needs `mu`? | Reason |
|---|---|---|
| `opampClient` | Yes | Written by worker (`reconnectClient`), nilled by `Stop()`, read by health/OnRemoteConfig/forwardCustomMessage. |
| `opampServer` | Yes | Written in `Start()`/`Stop()`, read by `forwardCustomMessage`. |
| `running` | Yes | Written in `Start()`/`Stop()`, read by `IsRunning()` and `reconnectClient` (shutdown guard). |
| `pendingCSR` | Yes | Written in `initAuth` (startup), read in `createAndStartClient` (startup + reconnect), cleared in `handleEnrollmentCertificate` (phase 1, on opamp-go goroutine). With the two-phase design, phase 1 clears `pendingCSR` under `mu.Lock()` concurrently with the worker calling `createAndStartClient`. **`createAndStartClient` must read `pendingCSR` under `mu.RLock()`** (currently unprotected at supervisor.go:466). |
| `healthCancel` | No | Written once in `Start()`, read once in `Stop()`. Safe because `Start()` and `Stop()` are never concurrent per the [Lifecycle Contract](#lifecycle-contract). |
| `commander` | No | Same as `healthCancel`: written once in `Start()`, read once in `Stop()`. Safe per [Lifecycle Contract](#lifecycle-contract). |
