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

package rest

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/context"
	"github.com/Graylog2/collector-sidecar/logger"
)

var (
	log                 = logger.Log()
	userAgent           = "Graylog Collector v" + common.CollectorVersion
	customRequestHeader = "sidecar"
)

const (
	defaultBaseURL = "http://127.0.0.1:9000/api/"
	mediaType      = "application/json"
)

type Client struct {
	client             *http.Client
	ApiToken           string
	BaseURL            *url.URL
	UserAgent          string
	onRequestCompleted RequestCompletionCallback
}

// RequestCompletionCallback defines the type of the request callback function
type RequestCompletionCallback func(*http.Request, *http.Response)

type Response struct {
	*http.Response
}

type ErrorResponse struct {
	// HTTP response that caused this error
	Response *http.Response
	// Error message
	Message string
}

func (r *ErrorResponse) Error() string {
	return fmt.Sprintf("%v %v: %d %v",
		r.Response.Request.Method, r.Response.Request.URL, r.Response.StatusCode, r.Message)
}

func NewHTTPClient(tlsConfig *tls.Config) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			Dial: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).Dial,
			TLSClientConfig:       tlsConfig,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			DisableCompression:    true,
		},
	}
}

func NewClient(httpClient *http.Client, ctx *context.Ctx) *Client {
	if httpClient == nil {
		log.Fatal("http client must not be nil")
	}

	baseURL, _ := url.Parse(defaultBaseURL)
	c := &Client{client: httpClient, ApiToken: ctx.UserConfig.ServerApiToken, BaseURL: baseURL, UserAgent: userAgent}

	return c
}

func (c *Client) NewRequest(method, urlStr string, params map[string]string, body interface{}) (*http.Request, error) {
	rel, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}

	if len(params) != 0 {
		for param, value := range params {
			q := rel.Query()
			q.Set(param, value)
			rel.RawQuery = q.Encode()
		}

	}

	rel.Path = path.Join(c.BaseURL.Path, rel.Path)
	u := c.BaseURL.ResolveReference(rel)

	buf := new(bytes.Buffer)
	if body != nil {
		err := json.NewEncoder(buf).Encode(body)
		if err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest(method, u.String(), buf)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", mediaType)
	req.Header.Add("Accept", mediaType)
	req.Header.Add("User-Agent", userAgent)
	req.Header.Add("X-Graylog-Sidecar-Version", common.CollectorVersion)
	req.Header.Add("X-Requested-By", customRequestHeader)
	req.SetBasicAuth(c.ApiToken, "token")
	return req, nil
}

func newResponse(r *http.Response) *Response {
	response := Response{Response: r}
	return &response
}

func CheckResponse(r *http.Response) error {
	if c := r.StatusCode; c >= 200 && c <= 299 {
		return nil
	}

	errorResponse := &ErrorResponse{Response: r}
	data, err := io.ReadAll(io.LimitReader(r.Body, 2048))
	if err == nil && len(data) > 0 {
		errorResponse.Message = string(data)
	}

	return errorResponse
}

func (c *Client) Do(req *http.Request, v interface{}) (*Response, error) {
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if c.onRequestCompleted != nil {
		c.onRequestCompleted(req, resp)
	}

	defer func() {
		io.Copy(ioutil.Discard, resp.Body)
		if rerr := resp.Body.Close(); err == nil {
			err = rerr
		}
	}()

	response := newResponse(resp)
	err = CheckResponse(resp)
	if err != nil {
		return response, err
	}

	if v != nil {
		if w, ok := v.(io.Writer); ok {
			_, err := io.Copy(w, resp.Body)
			if err != nil {
				return response, err
			}
		} else {
			err := json.NewDecoder(resp.Body).Decode(v)
			if err != nil {
				return response, err
			}
		}
	}

	return response, err
}
