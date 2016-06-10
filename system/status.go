package system

var (
	GlobalStatus = &Status{}
)

type Status struct {
	Status int `json:"status"`
	Message string `json:"message"`
}

func (status *Status) Set(state int, message string) {
	status.Status = state
	status.Message = message
}
