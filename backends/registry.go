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
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/logger"
)

var (
	log = logger.Log()
	// global store of available backends, like reported from Graylog server
	Store = &backendStore{backends: make(map[string]*Backend)}
)

type backendStore struct {
	backends map[string]*Backend
}

func (bs *backendStore) SetBackend(backend Backend) {
	bs.backends[backend.Id] = &backend
	executeParameters, err := common.Sprintf(backend.ExecuteParameters, backend.ConfigurationPath)
	if err != nil {
		log.Errorf("Invalid execute parameters, skip adding backend: %s", backend.Name)
		return
	}
	bs.backends[backend.Id].ExecuteParameters = executeParameters
	validationParameters, err := common.Sprintf(backend.ValidationParameters, backend.ConfigurationPath)
	if err != nil {
		log.Errorf("Invalid validation parameters, skip adding backend: %s", backend.Name)
		return
	}
	bs.backends[backend.Id].ValidationParameters = validationParameters
}

func (bs *backendStore) GetBackend(id string) *Backend {
	return bs.backends[id]
}

func (bs *backendStore) GetBackendById(id string) *Backend {
	for _, backend := range bs.backends {
		if backend.Id == id {
			return backend
		}
	}
	return nil
}

func (bs *backendStore) Update(backends []Backend) {
	if len(backends) != 0 {
		var activeIds []string
		for _, backend := range backends {
			activeIds = append(activeIds, backend.Id)

			// add new backend
			if bs.backends[backend.Id] == nil {
				bs.SetBackend(backend)
				// update if settings did change
			} else {
				if !bs.backends[backend.Id].EqualSettings(&backend) {
					bs.SetBackend(backend)
				}
			}
		}
		bs.Cleanup(activeIds)
	} else {
		bs.Cleanup([]string{})
	}
}

func (bs *backendStore) Cleanup(validBackendIds []string) {
	for _, backend := range bs.backends {
		if !common.IsInList(backend.Id, validBackendIds) {
			log.Debug("Cleaning up backend: " + backend.Name)
			delete(bs.backends, backend.Id)
		}
	}
}
