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
	"reflect"

	"github.com/Graylog2/collector-sidecar/helpers"
	"github.com/Graylog2/collector-sidecar/logger"
)

var log = logger.Log()

var (
	// global store of configuration assignments, [backendId-configurationID]ConfigurationId
	Store = &assignmentStore{assignments: make(map[string]string)}
)

type assignmentStore struct {
	assignments map[string]string
	// emptyStreak counts consecutive empty-assignment responses seen since
	// the assignments were last non-empty. Used to delay wiping locally
	// installed collectors until the empty response is confirmed by
	// confirmationThreshold consecutive polls.
	emptyStreak int
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

// WipePending reports whether an empty-assignment grace period is currently
// in progress, i.e. an empty assignments response has been seen but not yet
// confirmed by confirmationThreshold consecutive polls.
func (as *assignmentStore) WipePending() bool {
	return as.emptyStreak > 0
}

// Update applies a new assignments list from the server. If the list is
// empty and the store currently holds assignments, the wipe is delayed until
// confirmationThreshold consecutive empty responses have been seen in a row,
// to avoid removing locally installed collectors on a single spurious empty
// response (e.g. right after an outage clears).
func (as *assignmentStore) Update(assignments []ConfigurationAssignment, confirmationThreshold int) (modified bool, graceActive bool) {
	expandedAssignments := expandAssignments(assignments)

	beforeUpdate := make(map[string]string)
	for k, v := range as.assignments {
		beforeUpdate[k] = v
	}
	if len(expandedAssignments) != 0 {
		as.emptyStreak = 0
		var activeIds []string
		for backendId, assignment := range expandedAssignments {
			as.SetAssignment(backendId, assignment)
			activeIds = append(activeIds, backendId)
		}
		as.cleanup(activeIds)
	} else if len(as.assignments) > 0 {
		as.emptyStreak++
		if as.emptyStreak < confirmationThreshold {
			log.Warnf("Received empty assignments (%d/%d) - grace period active, not removing collectors",
				as.emptyStreak, confirmationThreshold)
			return false, true
		}
		log.Warn("Empty assignments confirmed after grace period - removing collectors")
		as.emptyStreak = 0
		as.cleanup([]string{})
	}
	return !reflect.DeepEqual(beforeUpdate, as.assignments), false
}

func (as *assignmentStore) cleanup(validBackendIds []string) {
	for backendId := range as.assignments {
		if !helpers.IsInList(backendId, validBackendIds) {
			delete(as.assignments, backendId)
		}
	}
}
