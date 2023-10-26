// Copyright (C) 2020 Graylog, Inc.

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

package api

import "github.com/hashicorp/go-version"

type GraylogVersion struct {
	*version.Version
}

func NewGraylogVersion(v string) (*GraylogVersion, error) {
	newVersion, err := version.NewVersion(v)
	if err != nil {
		return nil, err
	}
	return &GraylogVersion{newVersion}, nil
}

func (v *GraylogVersion) SupportsMultipleBackends() bool {
	// cannot use version.Constraints because of a bug in comparing pre-releases
	return (v.Version.Segments()[0] == 4 && v.Version.Segments()[1] >= 4) || v.Version.Segments()[0] >= 5
}

func (v *GraylogVersion) SupportsExtendedNodeDetails() bool {
	// cannot use version.Constraints because of a bug in comparing pre-releases
	return (v.Version.Segments()[0] == 4 && v.Version.Segments()[1] >= 4) || v.Version.Segments()[0] >= 5
}
