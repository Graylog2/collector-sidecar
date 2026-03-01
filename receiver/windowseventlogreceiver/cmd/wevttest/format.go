// Copyright (C)  2026 Graylog, Inc.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the Server Side Public License, version 1,
// as published by MongoDB, Inc.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// Server Side Public License for more details.
//
// You should have received a copy of the Server Side Public License
// along with this program. If not, see
// <http://www.mongodb.com/licensing/server-side-public-license>.
//
// SPDX-License-Identifier: SSPL-1.0

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"go.opentelemetry.io/collector/pdata/plog"
)

type outputFormat string

const (
	formatJSON outputFormat = "json"
	formatXML  outputFormat = "xml"
	formatOTel outputFormat = "otel"
)

func validFormat(s string) (outputFormat, error) {
	switch outputFormat(s) {
	case formatJSON, formatXML, formatOTel:
		return outputFormat(s), nil
	default:
		return "", fmt.Errorf("invalid format %q (valid: json, xml, otel)", s)
	}
}

type formatter struct {
	mu     sync.Mutex
	format outputFormat
	w      io.Writer
	ndjson bool // compact JSON, one event per line
}

func (f *formatter) writeLogs(ld plog.Logs) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch f.format {
	case formatOTel:
		return f.writeOTel(ld)
	case formatJSON:
		return f.writeJSON(ld)
	case formatXML:
		return f.writeXML(ld)
	default:
		return fmt.Errorf("unsupported format: %s", f.format)
	}
}

func (f *formatter) writeOTel(ld plog.Logs) error {
	m := &plog.JSONMarshaler{}
	data, err := m.MarshalLogs(ld)
	if err != nil {
		return err
	}
	var raw json.RawMessage = data
	pretty, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f.w, "%s\n", pretty)
	return err
}

func (f *formatter) writeJSON(ld plog.Logs) error {
	for i := range ld.ResourceLogs().Len() {
		rl := ld.ResourceLogs().At(i)
		for j := range rl.ScopeLogs().Len() {
			sl := rl.ScopeLogs().At(j)
			for k := range sl.LogRecords().Len() {
				lr := sl.LogRecords().At(k)
				raw := lr.Body().AsRaw()
				if raw == nil {
					continue
				}
				var data []byte
				var err error
				if f.ndjson {
					data, err = json.Marshal(raw)
				} else {
					data, err = json.MarshalIndent(raw, "", "  ")
				}
				if err != nil {
					return err
				}
				if _, err := fmt.Fprintf(f.w, "%s\n", data); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (f *formatter) writeXML(ld plog.Logs) error {
	for i := range ld.ResourceLogs().Len() {
		rl := ld.ResourceLogs().At(i)
		for j := range rl.ScopeLogs().Len() {
			sl := rl.ScopeLogs().At(j)
			for k := range sl.LogRecords().Len() {
				lr := sl.LogRecords().At(k)
				val, ok := lr.Attributes().Get("log.record.original")
				if !ok {
					if _, err := fmt.Fprintf(f.w, "<!-- no original XML available -->\n"); err != nil {
						return err
					}
					continue
				}
				if _, err := fmt.Fprintf(f.w, "%s\n", val.Str()); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
