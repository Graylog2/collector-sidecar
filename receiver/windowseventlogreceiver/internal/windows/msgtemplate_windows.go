// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package windows

import (
	"errors"
	"fmt"
	"unsafe"
)

// loadTemplates enumerates a provider's event metadata and caches compiled
// message templates. Called lazily on first template fallback for a provider.
func loadTemplates(pub Publisher, cache *templateCache) error {
	cache.loaded = true

	enumHandle, err := evtOpenEventMetadataEnum(pub.handle, 0)
	if err != nil {
		return fmt.Errorf("open event metadata enum: %w", err)
	}
	defer evtClose(enumHandle)

	for {
		metaHandle, err := evtNextEventMetadata(enumHandle, 0)
		if err != nil {
			if errors.Is(err, ErrorNoMoreItems) {
				break
			}
			return fmt.Errorf("next event metadata: %w", err)
		}

		loadSingleTemplate(pub, cache, metaHandle)
		evtClose(metaHandle)
	}

	return nil
}

// loadSingleTemplate extracts one event's metadata and compiles its message
// template into the cache. Errors are silently ignored per-event — a missing
// or unparseable template for one event should not prevent others from loading.
func loadSingleTemplate(pub Publisher, cache *templateCache, metaHandle uintptr) {
	eventID, err := getMetadataUint32(metaHandle, EvtEventMetadataEventID)
	if err != nil {
		return
	}

	version, err := getMetadataUint32(metaHandle, EvtEventMetadataEventVersion)
	if err != nil {
		return
	}

	messageID, err := getMetadataUint32(metaHandle, EvtEventMetadataEventMessageID)
	if err != nil {
		return
	}
	// 0xFFFFFFFF means no message is defined for this event.
	if messageID == 0xFFFFFFFF {
		return
	}

	templateStr, err := formatMessageID(pub, messageID)
	if err != nil {
		return
	}

	tmpl, err := compileTemplate(fmt.Sprintf("%d_v%d", eventID, version), templateStr)
	if err != nil {
		return
	}

	cache.put(eventID, uint8(version), tmpl)
}

// getMetadataUint32 reads a uint32 property from an event metadata handle.
// The property is returned as an EVT_VARIANT (16 bytes: 8-byte union + 4-byte count + 4-byte type).
func getMetadataUint32(metaHandle uintptr, propertyID uint32) (uint32, error) {
	// EVT_VARIANT is 16 bytes on both 32-bit and 64-bit Windows.
	var buf [16]byte
	_, err := evtGetEventMetadataProperty(metaHandle, propertyID, 0, 16, &buf[0])
	if err != nil {
		return 0, err
	}
	return *(*uint32)(unsafe.Pointer(&buf[0])), nil
}
