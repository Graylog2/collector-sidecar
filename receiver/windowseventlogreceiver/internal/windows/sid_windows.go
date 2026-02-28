// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package windows

import (
	"fmt"

	syswin "golang.org/x/sys/windows"
)

func defaultSIDLookup(sidStr string) (*SIDInfo, error) {
	sid, err := syswin.StringToSid(sidStr)
	if err != nil {
		return nil, fmt.Errorf("invalid SID %q: %w", sidStr, err)
	}
	account, domain, accType, err := sid.LookupAccount("")
	if err != nil {
		return nil, fmt.Errorf("lookup failed for %q: %w", sidStr, err)
	}
	return &SIDInfo{
		UserName: account,
		Domain:   domain,
		Type:     sidAccountTypeName(accType),
	}, nil
}

func sidAccountTypeName(t uint32) string {
	switch t {
	case 1:
		return "User"
	case 2:
		return "Group"
	case 3:
		return "Domain"
	case 4:
		return "Alias"
	case 5:
		return "WellKnownGroup"
	case 6:
		return "DeletedAccount"
	case 7:
		return "Invalid"
	case 8:
		return "Unknown"
	case 9:
		return "Computer"
	default:
		return "Unknown"
	}
}
