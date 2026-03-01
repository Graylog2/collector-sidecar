// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package windows

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"syscall"
	"time"

	conventions "go.opentelemetry.io/otel/semconv/v1.38.0"
	"go.uber.org/multierr"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/operator"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/operator/helper"
)

// Input is an operator that creates entries using the windows event log api.
type Input struct {
	helper.InputOperator
	bookmark                 *Bookmark
	buffer                   *Buffer
	channel                  string
	ignoreChannelErrors      bool
	query                    *string
	maxReads                 int
	currentMaxReads          int
	startAt                  string
	raw                      bool
	includeLogRecordOriginal bool
	excludeProviders         map[string]struct{}
	language                 uint32
	pollInterval             time.Duration
	persister                operator.Persister
	publisherCache           publisherCache
	cancel                   context.CancelFunc
	wg                       sync.WaitGroup
	subscription             Subscription
	subscriptionOpen         bool
	resolveSIDs              bool
	sidCacheSize             int
	sidCache                 *sidCache
	processEvent             func(context.Context, Event) error
}

// isNonTransientError checks if the error is likely non-transient.
func isNonTransientError(err error) bool {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return !isRecoverableError(uint32(errno))
	}
	return true // assume non-transient if we can't identify the error
}

// errIsChannelError checks if the error is specifically a channel-related error
// (e.g. channel not found), as opposed to other non-transient errors like access denied.
func errIsChannelError(err error) bool {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return isChannelError(uint32(errno))
	}
	return false
}

// Start will start reading events from a subscription.
func (i *Input) Start(persister operator.Persister) error {
	ctx, cancel := context.WithCancel(context.Background())
	i.cancel = cancel

	i.persister = persister

	i.bookmark = NewBookmark()
	offsetXML, err := i.getBookmarkOffset(ctx)

	if err != nil {
		_ = i.persister.Delete(ctx, i.getPersistKey())
	}

	if offsetXML != "" {
		if err := i.bookmark.Open(offsetXML); err != nil {
			return fmt.Errorf("failed to open bookmark: %w", err)
		}
	}

	i.publisherCache = newPublisherCache(i.language)

	if i.resolveSIDs {
		i.sidCache = newSIDCache(i.sidCacheSize, 5*time.Minute, defaultSIDLookup)
	}

	subscription := NewSubscription()
	if err := subscription.Open(i.startAt, i.channel, i.query, i.bookmark); err != nil {
		if isNonTransientError(err) {
			if !i.ignoreChannelErrors || !errIsChannelError(err) {
				return fmt.Errorf("failed to open local subscription: %w", err)
			}
			i.Logger().Warn("Channel not found, not starting", zap.Error(err))
			return nil
		}
		i.Logger().Warn("Transient error opening subscription, will retry with backoff", zap.Error(err))
		i.subscriptionOpen = false
	} else {
		i.subscriptionOpen = true
	}

	i.subscription = subscription
	i.wg.Add(1)
	go i.pollAndRead(ctx)

	return nil
}

// Stop will stop reading events from a subscription.
func (i *Input) Stop() error {
	if i.cancel != nil {
		i.cancel()
	}

	i.wg.Wait()

	var errs error
	if err := i.subscription.Close(); err != nil {
		errs = multierr.Append(errs, fmt.Errorf("failed to close subscription: %w", err))
	}

	if err := i.bookmark.Close(); err != nil {
		errs = multierr.Append(errs, fmt.Errorf("failed to close bookmark: %w", err))
	}

	if err := i.publisherCache.evictAll(); err != nil {
		errs = multierr.Append(errs, fmt.Errorf("failed to close publishers: %w", err))
	}

	return errs
}

func (i *Input) pollAndRead(ctx context.Context) {
	defer i.wg.Done()
	bo := newBackoff()

	for {
		if !i.subscriptionOpen {
			delay := bo.next()
			i.Logger().Info("Retrying subscription open", zap.Duration("delay", delay))
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			if err := i.subscription.Open(i.startAt, i.channel, i.query, i.bookmark); err != nil {
				if isNonTransientError(err) {
					if i.ignoreChannelErrors && errIsChannelError(err) {
						i.Logger().Warn("Channel not found on reopen, giving up", zap.Error(err))
					} else {
						i.Logger().Error("Non-transient error reopening subscription, stopping", zap.Error(err))
					}
					return
				}
				i.Logger().Warn("Transient error reopening subscription, will retry", zap.Error(err))
				continue
			}
			i.subscriptionOpen = true
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(i.pollInterval):
			switch i.read(ctx) {
			case readOK:
				bo.reset()
			case readFatal:
				return
			}
		}
	}
}

// readResult describes the outcome of a read cycle.
type readResult int

const (
	readOK    readResult = iota // at least one event was read
	readEmpty                   // no events (or recoverable error triggering reopen)
	readFatal                   // non-recoverable error, stop the receiver
)

// read drains all available events.
func (i *Input) read(ctx context.Context) readResult {
	hadEvents := false
	for {
		select {
		case <-ctx.Done():
			if hadEvents {
				return readOK
			}
			return readEmpty
		default:
			more, fatal := i.readBatch(ctx)
			if more {
				hadEvents = true
			}
			if fatal {
				return readFatal
			}
			if !more {
				if hadEvents {
					return readOK
				}
				return readEmpty
			}
		}
	}
}

// readBatch reads one batch of events. Returns (moreEvents, fatalError).
func (i *Input) readBatch(ctx context.Context) (bool, bool) {
	events, actualMaxReads, err := i.subscription.Read(i.currentMaxReads)

	// Update the current max reads if it changed
	if err == nil && actualMaxReads < i.currentMaxReads {
		i.currentMaxReads = actualMaxReads
		i.Logger().Debug("Encountered RPC_S_INVALID_BOUND, reduced batch size", zap.Int("current_batch_size", i.currentMaxReads), zap.Int("original_batch_size", i.maxReads))
	}

	if err != nil {
		// Handle not-open sentinel: trigger reopen
		if errors.Is(err, errSubscriptionHandleNotOpen) {
			i.Logger().Warn("Subscription handle not open, will reopen")
			i.subscriptionOpen = false
			return false, false
		}

		var errno syscall.Errno
		if errors.As(err, &errno) && isRecoverableError(uint32(errno)) {
			i.Logger().Warn("Recoverable read error, closing and reopening subscription", zap.Error(err))
			_ = i.subscription.Close()
			i.subscriptionOpen = false
			return false, false
		}

		// Non-recoverable: if it's a channel error and ignoreChannelErrors is set,
		// close and let the reopen loop handle it instead of stopping.
		if i.ignoreChannelErrors && errors.As(err, &errno) && isChannelError(uint32(errno)) {
			i.Logger().Warn("Channel error during read, closing and will retry", zap.Error(err))
			_ = i.subscription.Close()
			i.subscriptionOpen = false
			return false, false
		}
		i.Logger().Error("Fatal read error, stopping", zap.Error(err))
		return false, true
	}

	seenFailure := false
	for _, event := range events {
		if err := i.processEvent(ctx, event); err != nil {
			i.Logger().Error("process event", zap.Error(err))
			seenFailure = true
		} else if !seenFailure {
			i.updateBookmarkOffset(ctx, event)
		}
		event.Close()
	}

	return len(events) != 0, false
}

func (i *Input) getPublisherName(event Event) (name string, excluded bool) {
	providerName, err := event.GetPublisherName(i.buffer)
	if err != nil {
		i.Logger().Error("Failed to get provider name", zap.Error(err))
		return "", true
	}
	if _, exclude := i.excludeProviders[providerName]; exclude {
		return "", true
	}

	return providerName, false
}

func (i *Input) renderSimpleAndSend(ctx context.Context, event Event) error {
	simpleEvent, err := event.RenderSimple(i.buffer)
	if err != nil {
		return fmt.Errorf("render simple event: %w", err)
	}
	return i.sendEvent(ctx, simpleEvent)
}

// processEventWithoutRenderingInfo will process and send an event without rendering info.
func (i *Input) processEventWithoutRenderingInfo(ctx context.Context, event Event) error {
	if len(i.excludeProviders) == 0 {
		return i.renderSimpleAndSend(ctx, event)
	}
	if _, exclude := i.getPublisherName(event); exclude {
		return nil
	}
	return i.renderSimpleAndSend(ctx, event)
}

func (i *Input) processEventWithRenderingInfo(ctx context.Context, event Event) error {
	providerName, exclude := i.getPublisherName(event)
	if exclude {
		return nil
	}

	entry := i.publisherCache.getEntry(providerName)
	if entry == nil || !entry.publisher.Valid() {
		return i.renderSimpleAndSend(ctx, event)
	}

	return i.renderDeepWithFallback(ctx, event, entry)
}

// renderDeepWithFallback attempts deep rendering and falls back to template rendering.
//
// Fallback chain:
//  1. EvtFormatMessage (deep rendering) — primary path
//  2. Cached message template with Go text/template — secondary path
//  3. No message (simple XML rendering) — last resort
func (i *Input) renderDeepWithFallback(ctx context.Context, event Event, entry *publisherEntry) error {
	deepEvent, err := event.RenderDeep(i.buffer, entry.publisher)
	if err == nil {
		return i.sendEventWithEnrichment(ctx, deepEvent, entry)
	}
	// Deep rendering failed — render simple XML and try template fallback for message
	simpleEvent, simpleErr := event.RenderSimple(i.buffer)
	if simpleErr != nil {
		return multierr.Append(
			fmt.Errorf("render deep event: %w", err),
			fmt.Errorf("render simple event: %w", simpleErr),
		)
	}
	i.applyTemplateFallback(simpleEvent, entry)
	return i.sendEventWithEnrichment(ctx, simpleEvent, entry)
}

// applyTemplateFallback attempts to populate the event message using a cached
// message template when deep rendering has failed. Templates are loaded lazily
// from the provider's metadata on first fallback attempt.
func (i *Input) applyTemplateFallback(eventXML *EventXML, entry *publisherEntry) {
	if entry.templates == nil {
		return
	}

	// Lazy-load templates on first fallback for this provider
	if !entry.templates.loaded {
		if err := loadTemplates(entry.publisher, entry.templates); err != nil {
			i.Logger().Debug("Failed to load message templates", zap.Error(err))
		}
	}

	tmpl, ok := entry.templates.get(eventXML.EventID.ID, eventXML.Version)
	if !ok {
		return
	}

	params := extractEventParams(eventXML)
	msg, err := executeTemplate(tmpl, params)
	if err != nil {
		i.Logger().Debug("Failed to execute message template", zap.Error(err))
		return
	}

	eventXML.Message = msg
}

// sendEventWithEnrichment applies %%ID expansion and SID resolution before sending.
func (i *Input) sendEventWithEnrichment(ctx context.Context, eventXML *EventXML, entry *publisherEntry) error {
	// Apply %%ID expansion to message, EventData, and UserData values
	if entry != nil && entry.publisher.Valid() {
		resolver := newPublisherParamResolver(entry.publisher, entry.paramMessages)
		eventXML.Message = expandParamMessages(eventXML.Message, resolver)
		for idx := range eventXML.EventData.Data {
			eventXML.EventData.Data[idx].Value = expandParamMessages(eventXML.EventData.Data[idx].Value, resolver)
		}
		if eventXML.UserData != nil {
			for idx := range eventXML.UserData.Data {
				eventXML.UserData.Data[idx].Value = expandParamMessages(eventXML.UserData.Data[idx].Value, resolver)
			}
		}
	}

	return i.sendEvent(ctx, eventXML)
}

// sendEvent will send EventXML as an entry to the operator's output.
func (i *Input) sendEvent(ctx context.Context, eventXML *EventXML) error {
	var body any = eventXML.Original
	if !i.raw {
		bodyMap := formattedBody(eventXML)

		// Apply SID resolution
		if i.sidCache != nil && eventXML.Security != nil && eventXML.Security.UserID != "" {
			info, err := i.sidCache.resolve(eventXML.Security.UserID)
			if err == nil && info != nil {
				if sec, ok := bodyMap["security"].(map[string]any); ok {
					sec["user_name"] = info.UserName
					sec["domain"] = info.Domain
					sec["user_type"] = info.Type
				}
			}
		}

		body = bodyMap
	}

	e, err := i.NewEntry(body)
	if err != nil {
		return fmt.Errorf("create entry: %w", err)
	}

	ts, tsErr := parseTimestamp(eventXML.TimeCreated.SystemTime)
	if tsErr != nil {
		i.Logger().Warn("Timestamp parse failed, using current time", zap.Error(tsErr))
	}
	e.Timestamp = ts
	e.Severity = parseSeverity(eventXML.RenderedLevel, eventXML.Level)

	if i.includeLogRecordOriginal {
		e.AddAttribute(string(conventions.LogRecordOriginalKey), eventXML.Original)
	}

	return i.Write(ctx, e)
}

// getBookmarkOffset will get the bookmark xml from the offsets database.
func (i *Input) getBookmarkOffset(ctx context.Context) (string, error) {
	bytes, err := i.persister.Get(ctx, i.getPersistKey())
	return string(bytes), err
}

// updateBookmarkOffset will update the bookmark xml and save it in the offsets database.
func (i *Input) updateBookmarkOffset(ctx context.Context, event Event) {
	if err := i.bookmark.Update(event); err != nil {
		i.Logger().Error("Failed to update bookmark from event", zap.Error(err))
		return
	}

	bookmarkXML, err := i.bookmark.Render(i.buffer)
	if err != nil {
		i.Logger().Error("Failed to render bookmark xml", zap.Error(err))
		return
	}

	if err := i.persister.Set(ctx, i.getPersistKey(), []byte(bookmarkXML)); err != nil {
		i.Logger().Error("failed to set offsets", zap.Error(err))
		return
	}
}

func (i *Input) getPersistKey() string {
	if i.query != nil {
		return *i.query
	}

	return i.channel
}
