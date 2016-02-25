package graylog

type ResponseCollectorConfiguration struct {
	Inputs   []ResponseCollectorInput   `json:"inputs"`
	Outputs  []ResponseCollectorOutput  `json:"outputs"`
	Snippets []ResponseCollectorSnippet `json:"snippets"`
}

type ResponseCollectorInput struct {
	Id         string            `json:"input_id"`
	Backend    string            `json:"backend"`
	Type       string            `json:"type"`
	Name       string            `json:"name"`
	Properties map[string]string `json:"properties"`
	ForwardTo  string            `json:"forward_to"`
}

type ResponseCollectorOutput struct {
	Id         string            `json:"output_id"`
	Backend    string            `json:"backend"`
	Type       string            `json:"type"`
	Name       string            `json:"name"`
	Properties map[string]string `json:"properties"`
}

type ResponseCollectorSnippet struct {
	Id      string `json:"snippet_id"`
	Backend string `json:"backend"`
	Name    string `json:"name"`
	Value   string `json:"snippet"`
}
