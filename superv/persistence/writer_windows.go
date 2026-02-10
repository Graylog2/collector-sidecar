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

//go:build windows

package persistence

import (
	"fmt"
	"io/fs"
	"os"

	"github.com/Graylog2/collector-sidecar/superv/persistence/winrenameio"
)

func newStagedFile(filePath string, data []byte, perm os.FileMode) (StagedFile, error) {
	//goland:noinspection GoResourceLeak
	file, err := winrenameio.NewPendingFile(filePath, winrenameio.WithStaticPermissions(perm), winrenameio.WithReplaceOnClose())
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
	return &stagedWindowsFile{pendingFile: file, targetFile: filePath}, nil
}

var _ StagedFile = &stagedWindowsFile{}

type stagedWindowsFile struct {
	pendingFile    *winrenameio.PendingFile
	targetFile     string
	commitCallback func() error
}

func (stage *stagedWindowsFile) SetCommitCallback(commitCallback func() error) {
	if commitCallback != nil {
		stage.commitCallback = commitCallback
	}
}

func (stage *stagedWindowsFile) Commit() error {
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

func (stage *stagedWindowsFile) Cleanup() error {
	if stage.pendingFile != nil {
		if err := stage.pendingFile.Cleanup(); err != nil {
			return fmt.Errorf("cleanup staged file: %w", err)
		}
	}
	return nil
}

// writeFileAtomic writes content to path atomically on Windows using
// winrenameio, which calls MoveFileExW with MOVEFILE_REPLACE_EXISTING
// and MOVEFILE_WRITE_THROUGH. See the winrenameio package for details.
func writeFileAtomic(path string, content []byte, perm fs.FileMode) error {
	return winrenameio.WriteFile(path, content, perm)
}
