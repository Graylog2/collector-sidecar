package assignments

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

func (as *assignmentStore) GetAll() map[string]string {
	return as.assignments
}