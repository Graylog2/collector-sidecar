package api

import (
	"fmt"

	"gopkg.in/jmcvetta/napping.v3"
)

type ResponseCollectorConfiguration struct {
	Inputs  []ResponseCollectorInput  `json:"inputs"`
	Outputs []ResponseCollectorOutput `json:"outputs"`
}

type ResponseCollectorInput struct {
	Name       string
	Properties map[string]string
}

type ResponseCollectorOutput struct {
	Name       string
	Properties map[string]string
}

func RequestConfiguration(server string) (ResponseCollectorConfiguration, error) {
	s := napping.Session{}
	url := "http://" + server + ":8000/configuration"
	res := ResponseCollectorConfiguration{}

	resp, err := s.Get(url, nil, &res, nil)
	if err == nil && resp.Status() != 200 {
		fmt.Println("Bad response status from Graylog server")
		fmt.Printf("\t Status:  %v\n", resp.Status())
	}

	return res, err
}
