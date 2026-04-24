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

package supervisor

import "context"

// workFunc is a function to be executed by the serialized worker.
// No error return because the worker only handles fire-and-forget operations.
type workFunc func(ctx context.Context)

// runWorker processes work items sequentially until s.ctx is cancelled.
// Must be started before the OpAMP client (which triggers callbacks that
// enqueue work) and stopped after the OpAMP client.
func (s *Supervisor) runWorker() {
	defer s.workWg.Done()
	for {
		select {
		case fn := <-s.workQueue:
			fn(s.ctx)
		case <-s.ctx.Done():
			return
		}
	}
}

// enqueueWork sends a work item to the worker. Returns true if enqueued,
// false if ctx was cancelled or the worker has already stopped before the
// worker accepted the item.
// The unbuffered channel provides natural back-pressure: if the worker is
// busy, the caller blocks until the current item completes.
func (s *Supervisor) enqueueWork(ctx context.Context, fn workFunc) bool {
	select {
	case s.workQueue <- fn:
		return true
	case <-s.ctx.Done():
		return false
	case <-ctx.Done():
		return false
	}
}
