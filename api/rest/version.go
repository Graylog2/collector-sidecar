package rest

import "github.com/hashicorp/go-version"

type Version struct {
	*version.Version
}

func NewVersion(v string) (*Version, error) {
	newVersion, err := version.NewVersion(v)
	if err != nil {
		return nil, err
	}
	return &Version{newVersion}, nil
}

func (v Version) SupportsMultipleBackends() bool {
	compareWith, _ := version.NewVersion("4.4.0")
	return v.GreaterThanOrEqual(compareWith)
}

func (v *Version) SupportsExtendedNodeDetails() bool {
	compareWith, _ := version.NewVersion("4.4.0")
	return v.GreaterThanOrEqual(compareWith)
}
