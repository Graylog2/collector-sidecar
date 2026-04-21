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

package main

// Generate the OpenTelemetry Collector binary source code.
//go:generate go tool go.opentelemetry.io/collector/cmd/builder --config ./builder/builder-config.yaml --skip-compilation

// Modify the generated main.go and main_windows.go to add customization hooks.
//go:generate go run ./builder/mod/main.go -main-path ./builder/main.go -windows-main-path ./builder/main_windows.go

// Format the generated source code.
//go:generate gofmt -w -s -l ./builder

// Add license headers to the generated source code.
//go:generate go tool github.com/google/addlicense -f .license.template -ignore **/*.yaml ./builder
