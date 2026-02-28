// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package windows

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExpandParams_NoParams(t *testing.T) {
	resolver := func(id uint32) (string, bool) { return "", false }
	result := expandParamMessages("No parameters here", resolver)
	require.Equal(t, "No parameters here", result)
}

func TestExpandParams_SingleParam(t *testing.T) {
	resolver := func(id uint32) (string, bool) {
		if id == 1234 {
			return "Success", true
		}
		return "", false
	}
	result := expandParamMessages("Status: %%1234", resolver)
	require.Equal(t, "Status: Success", result)
}

func TestExpandParams_MultipleParams(t *testing.T) {
	resolver := func(id uint32) (string, bool) {
		switch id {
		case 100:
			return "Read", true
		case 200:
			return "Write", true
		default:
			return "", false
		}
	}
	result := expandParamMessages("Permissions: %%100 and %%200", resolver)
	require.Equal(t, "Permissions: Read and Write", result)
}

func TestExpandParams_UnresolvableLeftAsIs(t *testing.T) {
	resolver := func(id uint32) (string, bool) { return "", false }
	result := expandParamMessages("Status: %%9999", resolver)
	require.Equal(t, "Status: %%9999", result)
}

func TestExpandParams_SinglePercent_NotExpanded(t *testing.T) {
	resolver := func(id uint32) (string, bool) { return "X", true }
	result := expandParamMessages("50% complete", resolver)
	require.Equal(t, "50% complete", result)
}
