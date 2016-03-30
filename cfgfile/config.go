package cfgfile

import (
	"errors"
)

type SidecarConfig struct {
	ServerUrl   string `config:"server_url"`
	NodeId      string `config:"node_id"`
	CollectorId string `config:"collector_id"`
	Tags 	    []string `config:"tags"`
	LogPath     string `config:"log_path"`
	Backends    []SidecarBackend
}

type SidecarBackend struct {
	Name 		  string `config:"name"`
	Enabled           *bool  `config:"enabled"`
	BinaryPath        string `config:"binary_path"`
	ConfigurationPath string `config:"configuration_path"`

}

func (sc *SidecarConfig) GetIndexByName(name string) (int, error) {
	index := -1
	for i, backend := range sc.Backends {
		if backend.Name == name {
			index = i
		}
	}
	if index < 0 {
		return index, errors.New("Can not find configuration for: " + name)
	}
	return index, nil
}
