// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package windows

import "errors"

// EvtFormatMessageId is the flag for formatting a message by its ID.
const EvtFormatMessageId uint32 = 8

// formatMessageID formats a message string from a publisher by message ID.
func formatMessageID(pub Publisher, messageID uint32) (string, error) {
	buf := NewBuffer()
	bufferUsed, err := evtFormatMessage(pub.handle, 0, messageID, 0, 0, EvtFormatMessageId, buf.SizeWide(), buf.FirstByte())
	if err != nil {
		if errors.Is(err, ErrorInsufficientBuffer) {
			buf.UpdateSizeWide(*bufferUsed)
			bufferUsed, err = evtFormatMessage(pub.handle, 0, messageID, 0, 0, EvtFormatMessageId, buf.SizeWide(), buf.FirstByte())
		}
		if err != nil {
			return "", err
		}
	}

	bytes, err := buf.ReadWideChars(*bufferUsed)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

// newPublisherParamResolver creates a paramResolver that uses EvtFormatMessage
// with EvtFormatMessageId to resolve %%NNNN parameter messages, caching results
// in the publisher entry's paramMessages map.
func newPublisherParamResolver(pub Publisher, cache map[uint32]string) paramResolver {
	return func(id uint32) (string, bool) {
		if s, ok := cache[id]; ok {
			return s, s != "" // empty string = negative cache
		}
		s, err := formatMessageID(pub, id)
		if err != nil {
			cache[id] = "" // negative cache
			return "", false
		}
		cache[id] = s
		return s, true
	}
}
