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

package assignments

import (
	"github.com/Graylog2/collector-sidecar/common"
)

var (
	// global store of configuration assignments, [backendId]ConfigurationId
	Store = &assignmentStore{make(map[string]string)}
)

type assignmentStore struct {
	assignments map[string]string
}

type ConfigurationAssignment struct {
	BackendId       string `json:"collector_id"`
	ConfigurationId string `json:"configuration_id"`
}

func (as *assignmentStore) SetAssignment(assignment *ConfigurationAssignment) {
	if as.assignments[assignment.BackendId] != assignment.ConfigurationId {
		as.assignments[assignment.BackendId] = assignment.ConfigurationId
	}
}

func (as *assignmentStore) GetAssignment(backendId string) string {
	return as.assignments[backendId]
}

func (as *assignmentStore) Len() int {
	return len(as.assignments)
}

func (as *assignmentStore) GetAll() map[string]string {
	return as.assignments
}

func (as *assignmentStore) AssignedBackendIds() []string {
	var result []string
	for backendId := range as.assignments {
		result = append(result, backendId)
	}
	return result
}

func (as *assignmentStore) Update(assignments []ConfigurationAssignment) {
	if len(assignments) != 0 {
		var activeIds []string
		for _, assignment := range assignments {
			Store.SetAssignment(&assignment)
			activeIds = append(activeIds, assignment.BackendId)
		}
		Store.cleanup(activeIds)
	} else {
		Store.cleanup([]string{})
	}
}

func (as *assignmentStore) cleanup(validBackendIds []string) {
	for backendId := range as.assignments {
		if !common.IsInList(backendId, validBackendIds) {
			delete(as.assignments, backendId)
		}
	}
}
