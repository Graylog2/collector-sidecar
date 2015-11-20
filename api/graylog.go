package api

import (
	"fmt"
	"log"

	"gopkg.in/jmcvetta/napping.v3"
)

type ResponseNxConfiguration struct {
	Inputs  []ResponseNxInput  `json:"inputs"`
	Outputs []ResponseNxOutput `json:"outputs"`
}

type ResponseNxInput struct {
	Name       string
	Properties map[string]string
}

type ResponseNxOutput struct {
	Name       string
	Properties map[string]string
}

func RequestConfiguration(server string) ResponseNxConfiguration {
	s := napping.Session{}
	url := "http://" + server + ":8000/configuration"
	res := ResponseNxConfiguration{}

	resp, err := s.Get(url, nil, &res, nil)
	if err != nil {
		log.Fatal(err)
	}
	if resp.Status() != 200 {
		fmt.Println("Bad response status from Graylog server")
		fmt.Printf("\t Status:  %v\n", resp.Status())
	}

	return res
}
