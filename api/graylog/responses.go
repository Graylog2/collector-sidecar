package graylog

type ResponseCollectorConfiguration struct {
	Inputs   []ResponseCollectorInput   `json:"inputs"`
	Outputs  []ResponseCollectorOutput  `json:"outputs"`
	Snippets []ResponseCollectorSnippet `json:"snippets"`
}

type ResponseCollectorInput struct {
	Type       string            `json:"type"`
	Name       string            `json:"name"`
	Properties map[string]string `json:"properties"`
	ForwardTo  string            `json:"forward_to"`
}

type ResponseCollectorOutput struct {
	Type       string            `json:"type"`
	Name       string            `json:"name"`
	Properties map[string]string `json:"properties"`
}

type ResponseCollectorSnippet struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"snippet"`
}
