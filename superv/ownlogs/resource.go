// Copyright (C)  2026 Graylog, Inc.
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
//
// SPDX-License-Identifier: SSPL-1.0

package ownlogs

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.39.0"
)

// BuildResource creates an OTel resource with service identifying attributes
// for the supervisor's own log telemetry.
func BuildResource(serviceName, serviceVersion, instanceID string) *resource.Resource {
	attrs := []resource.Option{
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			// Required to let the server correctly process the log record.
			attribute.String("collector.receiver.type", "collector_log"),
		),
	}
	if serviceVersion != "" {
		attrs = append(attrs, resource.WithAttributes(semconv.ServiceVersion(serviceVersion)))
	}
	if instanceID != "" {
		attrs = append(attrs, resource.WithAttributes(semconv.ServiceInstanceID(instanceID)))
	}
	// Error is discarded because resource.New with only WithAttributes options
	// cannot fail in practice (no detectors, no schema URL conflicts).
	r, _ := resource.New(context.Background(), attrs...)
	return r
}
