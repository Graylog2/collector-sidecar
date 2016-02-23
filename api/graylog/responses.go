package graylog

type ResponseCollectorConfiguration struct {
	Inputs   []ResponseCollectorInput   `json:"inputs"`
	Outputs  []ResponseCollectorOutput  `json:"outputs"`
	Snippets []ResponseCollectorSnippet `json:"snippets"`
}

type ResponseCollectorInput struct {
	Backend    string            `json:"backend"`
	Type       string            `json:"type"`
	Name       string            `json:"name"`
	Properties map[string]string `json:"properties"`
	ForwardTo  string            `json:"forward_to"`
}

type ResponseCollectorOutput struct {
	Backend    string            `json:"backend"`
	Type       string            `json:"type"`
	Name       string            `json:"name"`
	Properties map[string]string `json:"properties"`
}

type ResponseCollectorSnippet struct {
	Backend string `json:"backend"`
	Name    string `json:"name"`
	Value   string `json:"snippet"`
}
