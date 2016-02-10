package graylog

type RegistrationRequest struct {
	NodeId      string            `json:"node_id"`
	NodeDetails map[string]string `json:"node_details"`
}
