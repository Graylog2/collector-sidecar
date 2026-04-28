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

//go:build !windows

package persistence

import (
	"fmt"
	"io/fs"
	"os"

	"github.com/google/renameio/v2"
)

func newStagedFile(filePath string, data []byte, perm os.FileMode) (StagedFile, error) {
	//goland:noinspection GoResourceLeak
	file, err := renameio.NewPendingFile(filePath, renameio.WithStaticPermissions(perm), renameio.WithReplaceOnClose())
	if err != nil {
		return nil, fmt.Errorf("creating staged file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Cleanup()
		return nil, fmt.Errorf("writing staged file: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Cleanup()
		return nil, fmt.Errorf("syncing staged file: %w", err)
	}
	return &stagedUnixFile{pendingFile: file}, nil
}

var _ StagedFile = &stagedUnixFile{}

type stagedUnixFile struct {
	pendingFile    *renameio.PendingFile
	commitCallback func() error
}

func (stage *stagedUnixFile) SetCommitCallback(commitCallback func() error) {
	if commitCallback != nil {
		stage.commitCallback = commitCallback
	}
}

func (stage *stagedUnixFile) Commit() error {
	if stage.pendingFile == nil {
		return fmt.Errorf("commit staged file: no pending file")
	}
	if err := stage.pendingFile.Close(); err != nil {
		_ = stage.pendingFile.Cleanup()
		return fmt.Errorf("close staged file: %w", err)
	}
	if stage.commitCallback != nil {
		if err := stage.commitCallback(); err != nil {
			return fmt.Errorf("exec commit callback: %w", err)
		}
	}
	return nil
}

func (stage *stagedUnixFile) Cleanup() error {
	if stage.pendingFile != nil {
		if err := stage.pendingFile.Cleanup(); err != nil {
			return fmt.Errorf("cleanup staged file: %w", err)
		}
	}
	return nil
}

// writeFileAtomic writes content to path atomically using renameio.
// This provides safe atomic writes on Unix systems.
func writeFileAtomic(path string, content []byte, perm fs.FileMode) error {
	if err := renameio.WriteFile(path, content, perm); err != nil {
		return fmt.Errorf("writing file atomically: %w", err)
	}
	return nil
}
