// This file is part of Graylog.
//
// Graylog is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Graylog is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with Graylog.  If not, see <http://www.gnu.org/licenses/>.

package backends

import (
	"github.com/Graylog2/collector-sidecar/logger"
	"github.com/Graylog2/collector-sidecar/common"
)

var (
	log = logger.Log()
	// global store of available backends, like reported from Graylog server
	Store   = &backendStore{backends: make(map[string]*Backend)}
)

type backendStore struct {
	backends map[string]*Backend
}

func (bs *backendStore) AddBackend(backend *Backend) {
	bs.backends[backend.Name] = backend
	executeParameters, err := common.SprintfList(backend.ExecuteParameters, backend.ConfigurationPath)
	if err != nil {
		log.Errorf("Invalid execute parameters, skip adding backend: %s", backend.Name)
		return
	}
	bs.backends[backend.Name].ExecuteParameters = executeParameters
	validationParameters, err := common.SprintfList(backend.ValidationParameters, backend.ConfigurationPath)
	if err != nil {
		log.Errorf("Invalid validation parameters, skip adding backend: %s", backend.Name)
		return
	}
	bs.backends[backend.Name].ValidationParameters = validationParameters
}

func (bs *backendStore) GetBackend(name string) *Backend {
	return bs.backends[name]
}

func (bs *backendStore) GetBackendById(id string) *Backend {
	for _, backend := range bs.backends {
		if backend.Id == id {
			return backend
		}
	}
	return nil
}
