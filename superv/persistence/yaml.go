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
	"fmt"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
)

func marshalStruct(delimiter string, data any) ([]byte, error) {
	k := koanf.New(delimiter)

	if err := k.Load(structs.Provider(data, "koanf"), nil); err != nil {
		return nil, fmt.Errorf("loading struct: %w", err)
	}

	buf, err := k.Marshal(yaml.Parser())
	if err != nil {
		return nil, fmt.Errorf("marshaling YAML: %w", err)
	}

	return buf, nil
}

// WriteYAMLFile marshals the given struct to YAML and writes it to the specified file path.
func WriteYAMLFile(delimiter string, filePath string, data any) error {
	buf, err := marshalStruct(delimiter, data)
	if err != nil {
		return err
	}

	if err := WriteFile(filePath, buf, 0o600); err != nil {
		return err
	}

	return nil
}

// StageYAMLFile marshals the given struct to YAML and stages it for atomic write to the specified file path.
func StageYAMLFile(delimiter string, filePath string, data any) (StagedFile, error) {
	buf, err := marshalStruct(delimiter, data)
	if err != nil {
		return nil, err
	}

	stagedFile, err := StageFile(filePath, buf)
	if err != nil {
		return nil, err
	}

	return stagedFile, nil
}

// LoadYAMLFile reads the specified YAML file and unmarshals its content into the provided struct pointer.
func LoadYAMLFile(delimiter string, filePath string, dest any) error {
	k := koanf.New(delimiter)

	if err := k.Load(file.Provider(filePath), yaml.Parser()); err != nil {
		return fmt.Errorf("loading YAML file: %w", err)
	}

	if err := k.Unmarshal("", dest); err != nil {
		return fmt.Errorf("unmarshaling YAML: %w", err)
	}

	return nil
}
