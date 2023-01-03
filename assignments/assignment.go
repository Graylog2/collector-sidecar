// Copyright (C) 2020 Graylog, Inc.
//
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

package assignments

import (
	"github.com/Graylog2/collector-sidecar/helpers"
	"reflect"
)

var (
	// global store of configuration assignments, [backendId-configurationID]ConfigurationId
	Store = &assignmentStore{make(map[string]string)}
)

type assignmentStore struct {
	assignments map[string]string
}

type ConfigurationAssignment struct {
	BackendId       string `json:"collector_id"`
	ConfigurationId string `json:"configuration_id"`
}

func (as *assignmentStore) SetAssignment(backendId string, configId string) {
	if as.assignments[backendId] != configId {
		as.assignments[backendId] = configId
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

func expandAssignments(assignments []ConfigurationAssignment) map[string]string {
	expandedAssignments := make(map[string]string)

	for _, assignment := range assignments {
		configId := assignment.ConfigurationId
		expandedAssignments[assignment.BackendId+"-"+configId] = configId
	}
	return expandedAssignments
}

func (as *assignmentStore) Update(assignments []ConfigurationAssignment) bool {
	expandedAssignments := expandAssignments(assignments)

	beforeUpdate := make(map[string]string)
	for k, v := range as.assignments {
		beforeUpdate[k] = v
	}
	if len(expandedAssignments) != 0 {
		var activeIds []string
		for backendId, assignment := range expandedAssignments {
			Store.SetAssignment(backendId, assignment)
			activeIds = append(activeIds, backendId)
		}
		Store.cleanup(activeIds)
	} else {
		Store.cleanup([]string{})
	}
	return !reflect.DeepEqual(beforeUpdate, as.assignments)
}

func (as *assignmentStore) cleanup(validBackendIds []string) {
	for backendId := range as.assignments {
		if !helpers.IsInList(backendId, validBackendIds) {
			delete(as.assignments, backendId)
		}
	}
}
