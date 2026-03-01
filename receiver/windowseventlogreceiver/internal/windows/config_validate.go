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
	if c.Channel == "" && c.Query == nil {
		return errors.New("either `channel` or `query` must be set")
	}
	if c.Query != nil && *c.Query == "" {
		return errors.New("the `query` field must not be empty when set")
	}
	if c.Channel != "" && c.Query != nil {
		return errors.New("either `channel` or `query` must be set, but not both")
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
