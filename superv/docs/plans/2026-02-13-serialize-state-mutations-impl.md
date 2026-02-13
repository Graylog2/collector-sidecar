# Serialize State-Mutating OpAMP Callbacks — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Serialize state-mutating OpAMP callbacks through a single worker goroutine to eliminate race conditions in client lifecycle management.

**Architecture:** Add a work queue (unbuffered channel) and worker goroutine to the Supervisor. State-mutating operations (`reconnectClient`, future `OnPackagesAvailable`, `OnCommand`) are enqueued as fire-and-forget work items. The `OnOpampConnectionSettings` callback is split into two phases: synchronous validation/staging (phase 1) and async reconnection on the worker (phase 2). `Stop()` is refactored to release `mu` before calling `opampClient.Stop()` to fix a pre-existing deadlock. The health goroutine gets an RLock snapshot to fix a nil-dereference race.

**Tech Stack:** Go 1.24, opamp-go, zap logging, stretchr/testify

**Design document:** `superv/docs/plans/2026-02-12-serialize-state-mutations-design.md`

---

## Task 1: Add `ReportsConnectionSettingsStatus` capability

This is a leaf change with no dependencies on other tasks. Add the capability flag so opamp-go sends `ConnectionSettingsStatus` upstream based on callback return values.

**Files:**
- Modify: `superv/opamp/client.go:65-128` (Capabilities struct + ToProto)
- Modify: `superv/opamp/client_test.go` (add test for new capability)
- Modify: `superv/supervisor/supervisor.go:438-446` (set flag in createAndStartClient)

**Step 1: Write the failing test**

Add a test in `superv/opamp/client_test.go` that verifies `ReportsConnectionSettingsStatus` maps to the correct protobuf bit:

```go
func TestCapabilities_ToProto_ReportsConnectionSettingsStatus(t *testing.T) {
	caps := Capabilities{
		ReportsConnectionSettingsStatus: true,
	}
	proto := caps.ToProto()

	expected := protobufs.AgentCapabilities_AgentCapabilities_ReportsStatus |
		protobufs.AgentCapabilities_AgentCapabilities_ReportsConnectionSettingsStatus
	require.Equal(t, expected, proto)
}
```

**Step 2: Run test to verify it fails**

Run: `cd superv && go test ./opamp/ -run TestCapabilities_ToProto_ReportsConnectionSettingsStatus -v`
Expected: FAIL — `ReportsConnectionSettingsStatus` field does not exist.

**Step 3: Add the capability field and ToProto mapping**

In `superv/opamp/client.go`:

1. Add `ReportsConnectionSettingsStatus bool` to the `Capabilities` struct (after `AcceptsOpAMPConnectionSettings`, line ~74).

2. Add the corresponding `ToProto()` mapping (after the `AcceptsOpAMPConnectionSettings` block, line ~110):

```go
if c.ReportsConnectionSettingsStatus {
	caps |= protobufs.AgentCapabilities_AgentCapabilities_ReportsConnectionSettingsStatus
}
```

**Step 4: Run test to verify it passes**

Run: `cd superv && go test ./opamp/ -run TestCapabilities_ToProto_ReportsConnectionSettingsStatus -v`
Expected: PASS

**Step 5: Set the capability in `createAndStartClient`**

In `superv/supervisor/supervisor.go:438-446`, add `ReportsConnectionSettingsStatus: true` alongside `AcceptsOpAMPConnectionSettings: true` in the `Capabilities` literal inside `createAndStartClient`.

**Step 6: Run all opamp tests**

Run: `cd superv && go test ./opamp/ -v -count=1`
Expected: All PASS

**Step 7: Commit**

```
feat(opamp): add ReportsConnectionSettingsStatus capability

Enables opamp-go to send ConnectionSettingsStatus upstream based on
callback return values.
```

---

## Task 2: Add worker infrastructure to Supervisor

Add the `workItem` type, worker fields, `runWorker` loop, and `enqueueWork` helper. No behavior changes yet — just infrastructure.

**Files:**
- Create: `superv/supervisor/worker.go`
- Modify: `superv/supervisor/supervisor.go:56-77` (add fields to struct)

**Step 1: Create `superv/supervisor/worker.go`**

```go
package supervisor

import "context"

// workItem wraps a function to be executed by the serialized worker.
// Uses func(ctx context.Context) (no error return) because the worker only
// handles fire-and-forget operations.
type workItem struct {
	fn func(ctx context.Context)
}

// runWorker processes work items sequentially until workCtx is cancelled.
// Must be started before the OpAMP client (which triggers callbacks that
// enqueue work) and stopped after the OpAMP client.
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

// enqueueWork sends a work item to the worker. Returns true if enqueued,
// false if ctx was cancelled before the worker accepted the item.
// The unbuffered channel provides natural back-pressure: if the worker is
// busy, the caller blocks until the current item completes.
func (s *Supervisor) enqueueWork(ctx context.Context, fn func(ctx context.Context)) bool {
	select {
	case s.workQueue <- workItem{fn: fn}:
		return true
	case <-ctx.Done():
		return false
	}
}
```

**Step 2: Add worker fields to the Supervisor struct**

In `superv/supervisor/supervisor.go`, add these fields to the `Supervisor` struct (after the `running` field, line ~73):

```go
// Serialized worker for state-mutating operations.
workQueue  chan workItem
workCtx    context.Context
workCancel context.CancelFunc
workWg     sync.WaitGroup
```

**Step 3: Verify compilation**

Run: `cd superv && go build ./supervisor/`
Expected: Success (no compilation errors)

**Step 4: Commit**

```
feat(supervisor): add worker infrastructure for serialized state mutations

Adds workItem type, runWorker loop, and enqueueWork helper. These are
not yet wired into Start/Stop — that happens in the next task.
```

---

## Task 3: Wire worker into Start/Stop lifecycle

Start the worker in `Start()` before `createAndStartClient()`. Refactor `Stop()` to: (1) release `mu` before calling `opampClient.Stop()` (fixing the pre-existing deadlock), (2) cancel `workCtx`, (3) wait for the worker.

**Files:**
- Modify: `superv/supervisor/supervisor.go:197-347` (Start and Stop methods)

**Step 1: Wire worker startup in `Start()`**

In `Start()`, after `s.mu.Lock()` / running check (line ~203) and **before** `createAndStartClient` (line ~268), add:

```go
// Initialize and start the serialized worker. Must be running before
// the OpAMP client starts, otherwise early callbacks block on the
// unbuffered channel with no consumer.
s.workQueue = make(chan workItem)
s.workCtx, s.workCancel = context.WithCancel(ctx)
s.workWg.Add(1)
go s.runWorker()
```

Also add cleanup of the worker in the error paths within `Start()` (e.g., if `createAndStartClient` fails, if `commander.Start` fails). In each error-return path after the worker is started, add:

```go
s.workCancel()
s.workWg.Wait()
```

**Step 2: Refactor `Stop()` — snapshot-and-nil pattern, release mu before stopping components**

Replace the current `Stop()` method (lines 308-347) with the design from the design doc:

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
	// Start() and Stop() are never concurrent (see Lifecycle Contract in design doc).
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

Key changes from current code:
- `s.mu` is released before calling `client.Stop()` (fixes deadlock where `Stop()` holds `mu` and waits for callbacks that need `mu`)
- `s.running = false` and nil-out of `opampClient`/`opampServer` happen atomically under `mu`
- Worker shutdown: cancel context first, wait for worker last
- `defer s.mu.Unlock()` is removed (explicit unlock before stopping components)

**Step 3: Verify compilation**

Run: `cd superv && go build ./supervisor/`
Expected: Success

**Step 4: Run existing tests**

Run: `cd superv && go test ./supervisor/... -v -count=1`
Expected: All PASS (or at least no new failures)

**Step 5: Commit**

```
feat(supervisor): wire worker into Start/Stop lifecycle

Start the worker before the OpAMP client so callbacks can enqueue work
immediately. Refactor Stop() to release mu before stopping the client,
fixing a pre-existing deadlock where Stop() holds mu waiting for
callbacks that also need mu.
```

---

## Task 4: Refactor `reconnectClient`

Replace the current `reconnectClient` with the design doc version: nil `s.opampClient` immediately under lock, skip stop if client is nil, check `s.running` before assigning new client.

**Files:**
- Modify: `superv/supervisor/supervisor.go:560-587` (reconnectClient method)

**Step 1: Replace `reconnectClient`**

Replace the current `reconnectClient` method (lines 560-587) with:

```go
func (s *Supervisor) reconnectClient(ctx context.Context, settings connection.Settings) error {
	s.mu.Lock()
	client := s.opampClient
	s.opampClient = nil // Nil immediately so concurrent readers see nil, not stopped client.
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
		// Best-effort stop: workCtx is already cancelled, so opamp-go's Stop()
		// returns ctx.Err() immediately.
		_ = newClient.Stop(ctx)
		return fmt.Errorf("supervisor stopped during reconnect")
	}
	s.opampClient = newClient
	s.mu.Unlock()

	return nil
}
```

Key changes from current code:
- Nils `s.opampClient` under lock **before** stopping old client (eliminates stopped-client window)
- Skips stop if client is already nil (supports rollback after failed reconnect)
- Checks `s.running` before assigning new client (prevents orphaned clients during shutdown)

**Step 2: Verify compilation**

Run: `cd superv && go build ./supervisor/`
Expected: Success

**Step 3: Run existing tests**

Run: `cd superv && go test ./supervisor/... -v -count=1`
Expected: All PASS

**Step 4: Commit**

```
fix(supervisor): eliminate stopped-client window in reconnectClient

Nil s.opampClient under lock before stopping the old client, so
concurrent readers see nil rather than a stopped client. Skip stop
when client is nil (supports rollback). Check s.running before
assigning to prevent orphaned clients during shutdown.
```

---

## Task 5: Fix health goroutine race

Apply the RLock snapshot pattern to the health goroutine so it doesn't access `s.opampClient` without synchronization.

**Files:**
- Modify: `superv/supervisor/supervisor.go:281-293` (health goroutine in Start)

**Step 1: Apply RLock snapshot pattern**

Replace the health goroutine (lines 281-293) with:

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

Key change: Added `s.mu.RLock()` / `s.mu.RUnlock()` around the `s.opampClient` read. The existing code (line ~288) accessed `s.opampClient` directly without any lock.

**Step 2: Verify compilation**

Run: `cd superv && go build ./supervisor/`
Expected: Success

**Step 3: Commit**

```
fix(supervisor): add RLock to health goroutine's opampClient access

The health goroutine accessed s.opampClient without synchronization,
risking nil dereference during reconnectClient. Apply the same RLock
snapshot pattern used by OnRemoteConfig and forwardCustomMessage.
```

---

## Task 6: Protect `pendingCSR` access in `createAndStartClient`

Per the mutex audit: `createAndStartClient` reads `s.pendingCSR` (line 466) without holding `mu`. With the two-phase design, phase 1 clears `pendingCSR` under `mu.Lock()` concurrently with the worker calling `createAndStartClient`.

**Files:**
- Modify: `superv/supervisor/supervisor.go:465-471` (pendingCSR access in createAndStartClient)

**Step 1: Add RLock around pendingCSR read**

Replace lines 465-471 in `createAndStartClient`:

```go
// We need this in the enrollment process
if len(s.pendingCSR) > 0 && client != nil {
```

with:

```go
// Read pendingCSR under RLock — handleEnrollmentCertificate (phase 1, on opamp-go
// goroutine) clears it under Lock concurrently with the worker calling this method.
s.mu.RLock()
csr := s.pendingCSR
s.mu.RUnlock()

if len(csr) > 0 {
```

And update the `RequestConnectionSettings` call to use `csr` instead of `s.pendingCSR`. Remove the `&& client != nil` check (client was just created, it's never nil here).

**Step 2: Verify compilation**

Run: `cd superv && go build ./supervisor/`
Expected: Success

**Step 3: Commit**

```
fix(supervisor): protect pendingCSR read with RLock in createAndStartClient

pendingCSR is cleared by handleEnrollmentCertificate under Lock on an
opamp-go goroutine, so createAndStartClient (called by the worker
during reconnect) must read it under RLock.
```

---

## Task 7: Split `OnOpampConnectionSettings` into two phases

This is the core behavioral change. Replace the current `go func()` goroutine approach with the two-phase design: synchronous `prepareConnectionSettings` (phase 1) + fire-and-forget `applyConnectionSettings` on the worker (phase 2).

**Files:**
- Modify: `superv/supervisor/supervisor.go:589-690` (replace handleConnectionSettings with prepareConnectionSettings + applyConnectionSettings)
- Modify: `superv/supervisor/supervisor.go:800-813` (rewire OnOpampConnectionSettings callback)

**Step 1: Add `pendingReconnect` type**

Add to `superv/supervisor/supervisor.go` (or a new file `superv/supervisor/connection_settings.go` if preferred, to keep supervisor.go from growing too large):

```go
// pendingReconnect holds state between prepareConnectionSettings (phase 1)
// and applyConnectionSettings (phase 2).
type pendingReconnect struct {
	newSettings connection.Settings
	oldSettings connection.Settings
	stagedFile  persistence.StagedFile
}
```

**Step 2: Write `prepareConnectionSettings` (phase 1)**

This runs synchronously in the callback. It performs all local, fast operations that can produce a meaningful success/failure status:

```go
// prepareConnectionSettings validates and stages new connection settings.
// Runs synchronously in the OnOpampConnectionSettings callback so the return
// value drives opamp-go's ConnectionSettingsStatus reporting.
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

Note: The heartbeat interval update that was in `handleConnectionSettings` is removed from this method. Per the design doc, opamp-go already updates the sender's heartbeat interval in `rcvOpampConnectionSettings` before invoking the callback, so the supervisor only needs to update its stored value. This is now handled by `SettingsChanged` which includes `HeartbeatInterval` in the `Settings` comparison and the updated settings struct.

**Step 3: Write `applyConnectionSettings` (phase 2)**

This runs on the worker, fire-and-forget:

```go
// applyConnectionSettings reconnects the OpAMP client with new settings.
// Runs on the serialized worker goroutine. On failure, rolls back to old settings.
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

	if err := pending.stagedFile.Commit(); err != nil {
		s.logger.Error("Failed to commit staged settings file", zap.Error(err))

		// Reconnect to old settings since persisting the new settings failed.
		if reconnectErr := s.reconnectClient(ctx, pending.oldSettings); reconnectErr != nil {
			s.logger.Error("Rollback after persistence error failed", zap.Error(reconnectErr))

			// If rollback fails, try new settings again for runtime consistency.
			if recoverErr := s.reconnectClient(ctx, pending.newSettings); recoverErr != nil {
				s.logger.Error("Recovery with new settings also failed", zap.Error(recoverErr))
			} else {
				s.connectionSettingsManager.SetCurrent(pending.newSettings)
				if persistErr := s.connectionSettingsManager.Persist(pending.newSettings); persistErr != nil {
					s.logger.Error("Recovery succeeded but persisting new settings still failed", zap.Error(persistErr))
				}
			}
		}
		return
	}

	s.logger.Info("Connection settings applied successfully")
}
```

**Step 4: Rewire the `OnOpampConnectionSettings` callback**

Replace lines 800-813 in `createOpAMPCallbacks`:

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
		// Enqueue failed (context cancelled during shutdown or opamp-go timeout).
		// Commit staged file so new settings are persisted for next restart.
		// The OpAMP spec says the server SHOULD NOT re-send unchanged settings.
		s.logger.Warn("Failed to enqueue connection settings apply (context cancelled), persisting for next restart")
		if commitErr := pending.stagedFile.Commit(); commitErr != nil {
			s.logger.Error("Failed to commit staged settings file", zap.Error(commitErr))
			if cleanupErr := pending.stagedFile.Cleanup(); cleanupErr != nil {
				s.logger.Error("Failed to clean up staged settings file", zap.Error(cleanupErr))
			}
			return fmt.Errorf("failed to persist settings for deferred apply: %w", commitErr)
		}

		// Restore old settings as in-memory baseline since we're not applying
		// the new settings at runtime. They're persisted on disk for next restart.
		s.connectionSettingsManager.SetCurrent(pending.oldSettings)
	}

	// TODO: Returning nil here reports APPLIED to the server, but reconnect has not
	// completed yet. This is optimistic — see "Status Reporting Limitations" in the
	// design doc. Once opamp-go exposes a public SetConnectionSettingsStatus API,
	// applyConnectionSettings should send late FAILED/APPLIED after async reconnect.
	return nil
},
```

**Step 5: Delete `handleConnectionSettings` and `reportConnectionSettingsStatus`**

These methods are now dead code:
- `handleConnectionSettings` (lines ~589-690) — replaced by `prepareConnectionSettings` + `applyConnectionSettings`
- `reportConnectionSettingsStatus` (lines ~730-754) — `SetConnectionSettingsStatus` is kept as a wrapper with a TODO, but the caller is removed since opamp-go now handles status from callback return values (with the `ReportsConnectionSettingsStatus` capability from Task 1)

**Step 6: Verify compilation**

Run: `cd superv && go build ./supervisor/`
Expected: Success

**Step 7: Run all tests**

Run: `cd superv && go test ./... -v -count=1`
Expected: All PASS

**Step 8: Commit**

```
feat(supervisor): two-phase connection settings with serialized worker

Split OnOpampConnectionSettings into prepareConnectionSettings (phase 1,
synchronous in callback for accurate error reporting) and
applyConnectionSettings (phase 2, fire-and-forget on worker for async
reconnect). This eliminates the untracked `go func()` goroutine that
raced with other callbacks and always returned nil, losing error
semantics.

Delete handleConnectionSettings and reportConnectionSettingsStatus
(replaced by two-phase design and ReportsConnectionSettingsStatus
capability respectively).
```

---

## Task 8: Remove heartbeat interval special-case from callback

The old `handleConnectionSettings` had a special heartbeat interval update that bypassed reconnection. With the two-phase design, `SettingsChanged` already includes heartbeat interval in its comparison, and the new settings (including updated heartbeat) flow through `reconnectClient` → `createAndStartClient` which sets the heartbeat interval on the new client.

Review and confirm that no separate heartbeat update logic is needed in `prepareConnectionSettings`. The design doc notes that opamp-go already updates the sender's heartbeat interval before the callback fires, so the supervisor's stored value only matters for future reconnects.

**Files:**
- Verify: `superv/supervisor/supervisor.go` — confirm no heartbeat-specific code remains outside `SettingsChanged`

**Step 1: Verify no heartbeat handling code remains**

Search for any remaining heartbeat-related code in `handleConnectionSettings` or the callback. It should all be gone after Task 7.

Run: `cd superv && grep -n "HeartbeatInterval\|heartbeat" supervisor/supervisor.go`
Expected: No matches in callback or connection settings handling code (only in settings struct/comparison).

**Step 2: Run all tests**

Run: `cd superv && go test ./... -v -count=1`
Expected: All PASS

**Step 3: Commit (if any changes needed)**

Only commit if this task revealed cleanup needed. Otherwise skip.

---

## Task 9: Final review and race detection

Run the full test suite with the race detector to verify no data races.

**Files:**
- All modified files from Tasks 1-8

**Step 1: Run all tests with race detector**

Run: `cd superv && go test ./... -race -count=1 -timeout 120s`
Expected: All PASS, no race conditions detected.

**Step 2: Run vet**

Run: `cd superv && go vet ./...`
Expected: No issues.

**Step 3: Review the complete diff**

Run: `git diff --stat` to see all changed files, then `git diff` to review.

Verify:
- No leftover `go func()` in `OnOpampConnectionSettings`
- `Stop()` releases `mu` before `client.Stop()`
- Health goroutine uses RLock snapshot
- `pendingCSR` read is protected with RLock
- `reconnectClient` nils `s.opampClient` before stopping old client
- `ReportsConnectionSettingsStatus` capability is set
- `reportConnectionSettingsStatus` and `handleConnectionSettings` are deleted

**Step 4: Commit (if any fixes needed)**

Only commit if this review revealed issues. Otherwise skip.
