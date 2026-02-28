// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package windows

import (
	"regexp"
	"strconv"
)

var paramPattern = regexp.MustCompile(`%%(\d+)`)

// paramResolver looks up a parameter message ID and returns the resolved
// string. Returns (resolved, true) on success or ("", false) if not found.
type paramResolver func(id uint32) (string, bool)

// expandParamMessages scans text for %%NNNN tokens and replaces them using
// the resolver. Unresolvable tokens are left as-is.
func expandParamMessages(text string, resolve paramResolver) string {
	return paramPattern.ReplaceAllStringFunc(text, func(match string) string {
		idStr := match[2:] // strip "%%"
		id, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			return match
		}
		if resolved, ok := resolve(uint32(id)); ok {
			return resolved
		}
		return match
	})
}
