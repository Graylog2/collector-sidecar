package api

import (
	"net/http"
	"strings"

	"github.com/Sirupsen/logrus"
	"gopkg.in/jmcvetta/napping.v3"
)

type ResponseCollectorConfiguration struct {
	Inputs  []ResponseCollectorInput  `json:"inputs"`
	Outputs []ResponseCollectorOutput `json:"outputs"`
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

type RegistrationRequest struct {
	NodeId      string            `json:"node_id"`
	NodeDetails map[string]string `json:"node_details"`
}

func RequestConfiguration(server string) (ResponseCollectorConfiguration, error) {
	host := strings.Replace(server, "12900", "8000", 1)
	s := napping.Session{}
	url := host + "/configuration"
	res := ResponseCollectorConfiguration{}

	resp, err := s.Get(url, nil, &res, nil)
	if err == nil && resp.Status() != 200 {
		logrus.Error("Bad response status from Graylog server: ", resp.Status(), err)
	}

	return res, err
}

func UpdateRegistration(server string) {
	s := napping.Session{}

	registration := RegistrationRequest{}
	registration.NodeId = "sidecar"
	registration.NodeDetails = make(map[string]string)
	registration.NodeDetails["operating_system"] = "Linux"

	h := http.Header{}
	h.Add("User-Agent", "Graylog Collector v0.0.1")
	h.Add("X-Graylog-Collector-Version", "0.0.1")

	r := napping.Request{
		Url:     server + "/system/collectors/3511945d-d16c-4000-9072-98ee1a77abb9",
		Method:  "PUT",
		Payload: registration,
		Header:  &h,
	}

	resp, err := s.Send(&r)
	if err == nil && resp.Status() != 202 {
		logrus.Error("Bad response from Graylog server: ", resp.Status(), err)
	}
}
