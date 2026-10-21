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

package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Graylog2/collector-sidecar/api/graylog"
	"github.com/Graylog2/collector-sidecar/api/rest"
	"github.com/Graylog2/collector-sidecar/cfgfile"
	"github.com/Graylog2/collector-sidecar/context"
)

type apiResponse struct {
	status int
	body   string
}

type apiTestCase struct {
	name        string
	response    apiResponse
	wantErr     bool
	notModified bool
}

func newTestContext(t *testing.T, response apiResponse) *context.Ctx {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.status)
		w.Write([]byte(response.body))
	}))
	t.Cleanup(server.Close)

	serverUrl, err := url.Parse(server.URL + "/api/")
	if err != nil {
		t.Fatal(err)
	}
	return &context.Ctx{
		ServerUrl:  serverUrl,
		NodeId:     "test-node",
		UserConfig: &cfgfile.SidecarConfig{ServerApiToken: "test-token"},
	}
}

// Responses a proxy or a restarting Graylog server may return. None of them must be
// treated as a valid (empty) answer, because that would remove all running collectors.
var invalidResponses = []apiTestCase{
	{name: "bad gateway", response: apiResponse{502, "<html>502 Bad Gateway</html>"}, wantErr: true},
	{name: "service unavailable", response: apiResponse{503, ""}, wantErr: true},
	{name: "empty body", response: apiResponse{200, ""}, wantErr: true},
	{name: "html body", response: apiResponse{200, "<html>Maintenance</html>"}, wantErr: true},
}

func TestUpdateRegistration(t *testing.T) {
	serverVersion, err := NewGraylogVersion("6.1.0")
	if err != nil {
		t.Fatal(err)
	}
	validBody := `{"assignments":[{"collector_id":"c1","configuration_id":"cfg1"}]}`
	testCases := append([]apiTestCase{
		{name: "valid", response: apiResponse{202, validBody}},
		{name: "not modified", response: apiResponse{304, ""}, notModified: true},
		{name: "accepted with empty body", response: apiResponse{202, ""}, wantErr: true},
		{name: "accepted with truncated body", response: apiResponse{202, `{"assignments":[{"collector_id":"c1"`}, wantErr: true},
		{name: "unexpected success status", response: apiResponse{200, validBody}, wantErr: true},
	}, invalidResponses...)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newTestContext(t, tc.response)
			response, err := UpdateRegistration(rest.NewHTTPClient(nil), "", ctx, serverVersion, &graylog.StatusRequest{})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got response %+v", response)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if response.NotModified != tc.notModified {
				t.Errorf("NotModified = %v, want %v", response.NotModified, tc.notModified)
			}
			if !tc.notModified && len(response.Assignments) != 1 {
				t.Errorf("expected 1 assignment, got %v", response.Assignments)
			}
		})
	}
}

func TestUpdateRegistrationUnmappableStatus(t *testing.T) {
	serverVersion, err := NewGraylogVersion("6.1.0")
	if err != nil {
		t.Fatal(err)
	}
	ctx := newTestContext(t, apiResponse{400, `{"message":"Unable to map property node_details"}`})
	ctx.UserConfig.SendStatus = true

	response, err := UpdateRegistration(rest.NewHTTPClient(nil), "", ctx, serverVersion, &graylog.StatusRequest{})
	if err == nil {
		t.Fatalf("expected error, got response %+v", response)
	}
	if ctx.UserConfig.SendStatus {
		t.Error("expected send_status to be disabled")
	}
}

func TestRequestBackendList(t *testing.T) {
	validBody := `{"collectors":[{"id":"c1","name":"filebeat"}]}`
	testCases := append([]apiTestCase{
		{name: "valid", response: apiResponse{200, validBody}},
		{name: "not modified", response: apiResponse{304, ""}, notModified: true},
		{name: "truncated body", response: apiResponse{200, `{"collectors":[{"id":"c1"`}, wantErr: true},
		{name: "unexpected success status", response: apiResponse{203, validBody}, wantErr: true},
	}, invalidResponses...)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newTestContext(t, tc.response)
			response, err := RequestBackendList(rest.NewHTTPClient(nil), "", ctx)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got response %+v", response)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if response.NotModified != tc.notModified {
				t.Errorf("NotModified = %v, want %v", response.NotModified, tc.notModified)
			}
			if !tc.notModified && len(response.Backends) != 1 {
				t.Errorf("expected 1 backend, got %v", response.Backends)
			}
		})
	}
}

func TestRequestConfiguration(t *testing.T) {
	validBody := `{"id":"cfg1","collector_id":"c1","template":"output: {}"}`
	testCases := append([]apiTestCase{
		{name: "valid", response: apiResponse{200, validBody}},
		{name: "not modified", response: apiResponse{304, ""}, notModified: true},
		{name: "truncated body", response: apiResponse{200, `{"id":"cfg1","template":"out`}, wantErr: true},
		{name: "unexpected success status", response: apiResponse{203, validBody}, wantErr: true},
	}, invalidResponses...)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newTestContext(t, tc.response)
			response, err := RequestConfiguration(rest.NewHTTPClient(nil), "cfg1", "", ctx)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got response %+v", response)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if response.NotModified != tc.notModified {
				t.Errorf("NotModified = %v, want %v", response.NotModified, tc.notModified)
			}
			if !tc.notModified && response.Template != "output: {}" {
				t.Errorf("unexpected template %q", response.Template)
			}
		})
	}
}

func TestRequestConfigurationNoContent(t *testing.T) {
	ctx := newTestContext(t, apiResponse{204, ""})
	response, err := RequestConfiguration(rest.NewHTTPClient(nil), "cfg1", "", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if response.Template != "" {
		t.Errorf("expected empty template, got %q", response.Template)
	}
}
