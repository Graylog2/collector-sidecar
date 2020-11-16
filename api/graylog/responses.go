// Copyright (C) 2020 Graylog, Inc.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the Server Side Public License, version 1,
// as published by MongoDB, Inc.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// Server Side Public License for more details.
//
// You should have received a copy of the Server Side Public License
// along with this program. If not, see
// <http://www.mongodb.com/licensing/server-side-public-license>.

package graylog

import "github.com/Graylog2/collector-sidecar/assignments"

type ResponseCollectorRegistration struct {
	Configuration         ResponseCollectorRegistrationConfiguration `json:"configuration"`
	ConfigurationOverride bool                                       `json:"configuration_override"`
	CollectorActions      []ResponseCollectorAction                  `json:"actions,omitempty"`
	Assignments           []assignments.ConfigurationAssignment      `json:"assignments,omitempty"`
	Checksum              string                                     //Etag of the response
	NotModified           bool
}

type ResponseCollectorAction struct {
	BackendId  string                 `json:"collector_id"`
	Properties map[string]interface{} `json:"properties"`
}

type ResponseCollectorRegistrationConfiguration struct {
	UpdateInterval int  `json:"update_interval"`
	SendStatus     bool `json:"send_status"`
}

type ResponseBackendList struct {
	Backends    []ResponseCollectorBackend `json:"collectors"`
	Checksum    string                     //Etag of the response
	NotModified bool
}

type ResponseCollectorBackend struct {
	Id                    string `json:"id"`
	Name                  string `json:"name"`
	ServiceType           string `json:"service_type"`
	OperatingSystem       string `json:"node_operating_system"`
	ExecutablePath        string `json:"executable_path"`
	ConfigurationFileName string `json:"configuration_file_name"`
	ExecuteParameters     string `json:"execute_parameters"`
	ValidationParameters  string `json:"validation_parameters"`
}

type ResponseCollectorConfiguration struct {
	ConfigurationId string `json:"id"`
	BackendId       string `json:"collector_id"`
	Name            string `json:"name"`
	Template        string `json:"template"`
	Checksum        string //Etag of the response
	NotModified     bool
}
