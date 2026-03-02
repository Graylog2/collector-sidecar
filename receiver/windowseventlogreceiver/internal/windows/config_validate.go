// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package windows

import (
	"encoding/xml"
	"errors"
	"fmt"
)

// validateConfig validates the Config fields that don't require Windows APIs.
func validateConfig(c *Config) error {
	// Canonicalize channel_list in-place before validation so that
	// whitespace-only or duplicate entries don't pass validation
	// only to produce an empty list at runtime.
	c.ChannelList = canonicalizeChannelList(c.ChannelList)

	sources := 0
	if c.Channel != "" {
		sources++
	}
	if len(c.ChannelList) > 0 {
		sources++
	}
	if c.Query != nil {
		sources++
	}

	if sources == 0 {
		return errors.New("either `channel`, `channel_list`, or `query` must be set")
	}
	if sources > 1 {
		return errors.New("only one of `channel`, `channel_list`, or `query` may be set")
	}
	if c.Query != nil && *c.Query == "" {
		return errors.New("the `query` field must not be empty when set")
	}
	if c.MaxReads < 1 {
		return errors.New("the `max_reads` field must be greater than zero")
	}
	if c.StartAt != "end" && c.StartAt != "beginning" {
		return errors.New("the `start_at` field must be set to `beginning` or `end`")
	}
	if c.SIDCacheSize < 0 {
		return errors.New("the `sid_cache_size` field must not be negative")
	}
	if c.Query != nil {
		if err := validateQueryXML(*c.Query); err != nil {
			return err
		}
	}
	return nil
}

// validateQueryXML checks that the query string is valid XML.
func validateQueryXML(query string) error {
	if err := xml.Unmarshal([]byte(query), &struct {
		XMLName xml.Name
	}{}); err != nil {
		return fmt.Errorf("invalid xml_query: %w", err)
	}
	return nil
}
