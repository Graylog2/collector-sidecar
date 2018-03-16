package assignments

import "github.com/Graylog2/collector-sidecar/common"

var (
	// global store of configuration assignments, [backendId]ConfigurationId
	Store   = &assignmentStore{make(map[string]string)}
)

type assignmentStore struct {
	assignments map[string]string
}

type ConfigurationAssignment struct {
	BackendId       string `json:"backend_id"`
	ConfigurationId string `json:"configuration_id"`
}

func (as *assignmentStore) SetAssignment(assignment *ConfigurationAssignment) {
	as.assignments[assignment.BackendId] = assignment.ConfigurationId
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

func (as *assignmentStore) CleanStore(validBackendIds []string) {
	for backendId := range as.assignments {
		if !common.IsInList(backendId, validBackendIds) {
			delete(as.assignments, backendId)
		}
	}
}