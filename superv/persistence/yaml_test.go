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

package persistence

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type yamlTestData struct {
	Name string `koanf:"name"`
	Port int    `koanf:"port"`
}

func TestWriteAndLoadYAMLFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "settings.yaml")

	expected := yamlTestData{
		Name: "collector",
		Port: 4317,
	}

	err := WriteYAMLFile(".", filePath, expected)
	require.NoError(t, err)

	var actual yamlTestData
	err = LoadYAMLFile(".", filePath, &actual)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func TestLoadYAMLFileRequiresPointerDestination(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "settings.yaml")

	err := WriteYAMLFile(".", filePath, yamlTestData{Name: "collector", Port: 4317})
	require.NoError(t, err)

	err = LoadYAMLFile(".", filePath, yamlTestData{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "pointer")
}

func TestStageYAMLFileCommitWritesData(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "settings.yaml")
	expected := yamlTestData{
		Name: "staged",
		Port: 9000,
	}

	stage, err := StageYAMLFile(".", filePath, expected)
	require.NoError(t, err)

	var beforeCommit yamlTestData
	err = LoadYAMLFile(".", filePath, &beforeCommit)
	require.ErrorIs(t, err, os.ErrNotExist)

	err = stage.Commit()
	require.NoError(t, err)

	var actual yamlTestData
	err = LoadYAMLFile(".", filePath, &actual)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func TestStageYAMLFileCleanupKeepsExistingFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "settings.yaml")

	original := yamlTestData{
		Name: "current",
		Port: 4317,
	}
	err := WriteYAMLFile(".", filePath, original)
	require.NoError(t, err)

	stage, err := StageYAMLFile(".", filePath, yamlTestData{Name: "candidate", Port: 9000})
	require.NoError(t, err)

	err = stage.Cleanup()
	require.NoError(t, err)

	var loaded yamlTestData
	err = LoadYAMLFile(".", filePath, &loaded)
	require.NoError(t, err)
	require.Equal(t, original, loaded)
}

func TestLoadYAMLFileMissingFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "missing.yaml")

	var loaded yamlTestData
	err := LoadYAMLFile(".", filePath, &loaded)
	require.ErrorIs(t, err, os.ErrNotExist)
}
