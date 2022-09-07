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

func (v GraylogVersion) SupportsMultipleBackends() bool {
	// cannot use version.Constraints because of a bug in comparing pre-releases
	return v.Version.Segments()[0] >= 4 && v.Version.Segments()[1] >= 4
}

func (v *GraylogVersion) SupportsExtendedNodeDetails() bool {
	// cannot use version.Constraints because of a bug in comparing pre-releases
	return v.Version.Segments()[0] >= 4 && v.Version.Segments()[1] >= 4
}
