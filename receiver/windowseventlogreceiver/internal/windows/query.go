// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package windows

import (
	"encoding/xml"
	"sort"
	"strings"
)

// queryList represents a Windows Event Log structured query.
type queryList struct {
	XMLName xml.Name     `xml:"QueryList"`
	Query   queryElement `xml:"Query"`
}

type queryElement struct {
	ID      string          `xml:"Id,attr"`
	Selects []selectElement `xml:"Select"`
}

type selectElement struct {
	Path  string `xml:"Path,attr"`
	Value string `xml:",chardata"`
}

// buildQueryFromChannels builds a structured XML query that subscribes
// to all the given channels with a wildcard selector.
func buildQueryFromChannels(channels []string) string {
	q := queryList{
		Query: queryElement{
			ID:      "0",
			Selects: make([]selectElement, len(channels)),
		},
	}
	for i, ch := range channels {
		q.Query.Selects[i] = selectElement{Path: ch, Value: "*"}
	}
	var b strings.Builder
	enc := xml.NewEncoder(&b)
	_ = enc.Encode(q)
	return b.String()
}

// channelListPersistKey returns a deterministic persist key for a channel list.
// It lowercases, sorts, and joins channel names with newlines.
func channelListPersistKey(channels []string) string {
	normalized := make([]string, len(channels))
	for idx, ch := range channels {
		normalized[idx] = strings.ToLower(ch)
	}
	sort.Strings(normalized)
	return strings.Join(normalized, "\n")
}
