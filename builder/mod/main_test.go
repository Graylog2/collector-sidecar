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

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Sample source code that mimics the generated OTel Collector main.go structure
const validMainGo = `package main

import (
	"go.opentelemetry.io/collector/otelcol"
)

func runInteractive(params otelcol.CollectorSettings) error {
	cmd := otelcol.NewCommand(params)
	return cmd.Execute()
}
`

// Source without runInteractive function
const mainGoWithoutRunInteractive = `package main

func main() {
	println("hello")
}
`

// Source with runInteractive but without cmd assignment
const mainGoWithoutCmdAssignment = `package main

func runInteractive(params int) error {
	result := doSomething(params)
	return result
}
`

func TestAddCustomizationCalls(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainPath, []byte(validMainGo), 0644); err != nil {
		t.Fatal(err)
	}

	if err := addCustomizationCalls(mainPath); err != nil {
		t.Fatalf("addCustomizationCalls failed: %v", err)
	}

	content, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the callbacks were inserted
	if !strings.Contains(string(content), "customizeSettings(&params)") {
		t.Errorf("expected customizeSettings(&params) call to be inserted")
	}
	if !strings.Contains(string(content), "customizeCommand(&params, cmd)") {
		t.Errorf("expected customizeCommand(&params, cmd) call to be inserted")
	}

	// Check that the order is correct: customizeSettings before cmd, customizeCommand after
	settingsIdx := strings.Index(string(content), "customizeSettings(&params)")
	cmdIdx := strings.Index(string(content), "cmd := otelcol.NewCommand(params)")
	commandIdx := strings.Index(string(content), "customizeCommand(&params, cmd)")

	if settingsIdx == -1 || cmdIdx == -1 || commandIdx == -1 {
		t.Fatal("could not find expected statements in modified file")
	}

	if settingsIdx >= cmdIdx {
		t.Errorf("customizeSettings should appear before cmd assignment")
	}
	if commandIdx <= cmdIdx {
		t.Errorf("customizeCommand should appear after cmd assignment")
	}
}

func TestAddCustomizationCallsIdempotent(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainPath, []byte(validMainGo), 0644); err != nil {
		t.Fatal(err)
	}

	// Run twice
	if err := addCustomizationCalls(mainPath); err != nil {
		t.Fatalf("first addCustomizationCalls failed: %v", err)
	}
	if err := addCustomizationCalls(mainPath); err != nil {
		t.Fatalf("second addCustomizationCalls failed: %v", err)
	}

	content, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatal(err)
	}

	// Count occurrences - should only appear once each
	settingsCount := strings.Count(string(content), "customizeSettings(&params)")
	commandCount := strings.Count(string(content), "customizeCommand(&params, cmd)")

	if settingsCount != 1 {
		t.Errorf("expected exactly 1 customizeSettings call, found %d", settingsCount)
	}
	if commandCount != 1 {
		t.Errorf("expected exactly 1 customizeCommand call, found %d", commandCount)
	}
}

func TestAddCustomizationCallsMissingRunInteractive(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainPath, []byte(mainGoWithoutRunInteractive), 0644); err != nil {
		t.Fatal(err)
	}

	err := addCustomizationCalls(mainPath)
	if err == nil {
		t.Fatal("expected error when runInteractive function is missing")
	}
	if !strings.Contains(err.Error(), "could not find cmd assignment") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestAddCustomizationCallsMissingCmdAssignment(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainPath, []byte(mainGoWithoutCmdAssignment), 0644); err != nil {
		t.Fatal(err)
	}

	err := addCustomizationCalls(mainPath)
	if err == nil {
		t.Fatal("expected error when cmd assignment is missing")
	}
	if !strings.Contains(err.Error(), "could not find cmd assignment") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestAddCustomizationCallsNonExistentFile(t *testing.T) {
	err := addCustomizationCalls("/nonexistent/path/main.go")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestAddCustomizationCallsInvalidGoFile(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainPath, []byte("this is not valid go code {{{{"), 0644); err != nil {
		t.Fatal(err)
	}

	err := addCustomizationCalls(mainPath)
	if err == nil {
		t.Fatal("expected error for invalid Go file")
	}
	if !strings.Contains(err.Error(), "failed to parse file") {
		t.Errorf("unexpected error message: %v", err)
	}
}

const validMainWindowsGo = `package main

import (
	"errors"
	"fmt"
	"go.opentelemetry.io/collector/otelcol"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
)

func run(params otelcol.CollectorSettings) error {
	// No need to supply service name when startup is invoked through
	// the Service Control Manager directly.
	if err := svc.Run("", otelcol.NewSvcHandler(params)); err != nil {
		if errors.Is(err, windows.ERROR_FAILED_SERVICE_CONTROLLER_CONNECT) {
			return runInteractive(params)
		}
		return fmt.Errorf("failed to start collector server: %w", err)
	}
	return nil
}
`

const mainWindowsGoWithoutRun = `package main

func main() {
	println("hello")
}
`

func TestAddSupervisorDispatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main_windows.go")
	if err := os.WriteFile(path, []byte(validMainWindowsGo), 0644); err != nil {
		t.Fatal(err)
	}

	if err := addSupervisorDispatch(path); err != nil {
		t.Fatalf("addSupervisorDispatch failed: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(content), "maybeSupervisorService(params)") {
		t.Errorf("expected maybeSupervisorService(params) call to be inserted")
	}

	dispatchIdx := strings.Index(string(content), "maybeSupervisorService(params)")
	svcRunIdx := strings.Index(string(content), "svc.Run")
	if dispatchIdx >= svcRunIdx {
		t.Errorf("maybeSupervisorService should appear before svc.Run")
	}
}

func TestAddSupervisorDispatchIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main_windows.go")
	if err := os.WriteFile(path, []byte(validMainWindowsGo), 0644); err != nil {
		t.Fatal(err)
	}

	if err := addSupervisorDispatch(path); err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if err := addSupervisorDispatch(path); err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	count := strings.Count(string(content), "maybeSupervisorService(params)")
	if count != 1 {
		t.Errorf("expected exactly 1 maybeSupervisorService call, found %d", count)
	}
}

func TestAddSupervisorDispatchMissingRunFunc(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main_windows.go")
	if err := os.WriteFile(path, []byte(mainWindowsGoWithoutRun), 0644); err != nil {
		t.Fatal(err)
	}

	err := addSupervisorDispatch(path)
	if err == nil {
		t.Fatal("expected error when run function is missing")
	}
	if !strings.Contains(err.Error(), "could not find run function") {
		t.Errorf("unexpected error message: %v", err)
	}
}
