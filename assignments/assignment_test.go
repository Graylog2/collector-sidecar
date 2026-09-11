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

package assignments

import "testing"

// newStoreWithAssignment returns a fresh store pre-populated with one
// assignment, isolated from the package-level Store singleton.
func newStoreWithAssignment() *assignmentStore {
	return &assignmentStore{assignments: map[string]string{"backend-1": "config-1"}}
}

// TestUpdateGracePeriodSuppressesFirstEmptyResponse guards the fix for the
// customer-reported bug where a single empty assignments response (e.g. right
// after a Graylog outage clears) caused the sidecar to immediately uninstall
// all locally managed collectors.
func TestUpdateGracePeriodSuppressesFirstEmptyResponse(t *testing.T) {
	as := newStoreWithAssignment()

	modified, graceActive := as.Update([]ConfigurationAssignment{}, 3)

	if modified {
		t.Fatalf("first empty response within grace period should report unmodified")
	}
	if !graceActive {
		t.Fatalf("first empty response within grace period should report graceActive")
	}
	if as.Len() != 1 {
		t.Fatalf("assignment should survive the grace period, got %d assignments", as.Len())
	}
}

// TestUpdateWipesAfterConfirmationThreshold verifies that once the empty
// response is confirmed by K consecutive polls, the assignment is actually
// removed.
func TestUpdateWipesAfterConfirmationThreshold(t *testing.T) {
	as := newStoreWithAssignment()
	threshold := 3

	for i := 0; i < threshold-1; i++ {
		as.Update([]ConfigurationAssignment{}, threshold)
		if as.Len() != 1 {
			t.Fatalf("assignment should still survive at streak %d/%d, got %d assignments", i+1, threshold, as.Len())
		}
	}

	modified, graceActive := as.Update([]ConfigurationAssignment{}, threshold)

	if !modified {
		t.Fatalf("confirmed empty response should report modified")
	}
	if graceActive {
		t.Fatalf("confirmed empty response should no longer report graceActive")
	}
	if as.Len() != 0 {
		t.Fatalf("assignment should be removed after %d confirmed empty responses, got %d assignments", threshold, as.Len())
	}
}

// TestUpdateResetsStreakOnNonEmptyResponse verifies that a real assignment
// arriving mid-grace-period resets the streak, so a later isolated empty
// response starts a fresh grace period instead of immediately wiping.
func TestUpdateResetsStreakOnNonEmptyResponse(t *testing.T) {
	as := newStoreWithAssignment()
	threshold := 3

	as.Update([]ConfigurationAssignment{}, threshold) // streak = 1
	// non-empty response resets the streak
	as.Update([]ConfigurationAssignment{{BackendId: "backend-1", ConfigurationId: "config-1"}}, threshold)

	_, graceActive := as.Update([]ConfigurationAssignment{}, threshold) // streak should now be 1, not 2

	if !graceActive {
		t.Fatalf("a fresh grace period should be active after the streak reset")
	}
	if as.Len() != 1 {
		t.Fatalf("assignment should survive: streak should have reset after the non-empty response, got %d assignments", as.Len())
	}
}

// TestUpdateThresholdOfOneWipesImmediately preserves the pre-grace-period
// behavior for operators who configure unassign_confirmation_count: 1.
func TestUpdateThresholdOfOneWipesImmediately(t *testing.T) {
	as := newStoreWithAssignment()

	modified, graceActive := as.Update([]ConfigurationAssignment{}, 1)

	if !modified {
		t.Fatalf("threshold of 1 should wipe on the first empty response")
	}
	if graceActive {
		t.Fatalf("threshold of 1 should never report graceActive")
	}
	if as.Len() != 0 {
		t.Fatalf("want 0 assignments with threshold 1, got %d", as.Len())
	}
}

// TestUpdateAlreadyEmptyStaysEmpty verifies that a store with no assignments
// is unaffected by empty responses (nothing to protect, no streak to track).
func TestUpdateAlreadyEmptyStaysEmpty(t *testing.T) {
	as := &assignmentStore{assignments: map[string]string{}}

	modified, graceActive := as.Update([]ConfigurationAssignment{}, 3)

	if modified {
		t.Fatalf("update from empty to empty should report unmodified")
	}
	if graceActive {
		t.Fatalf("an already-empty store has nothing to protect, want graceActive false")
	}
	if as.Len() != 0 {
		t.Fatalf("want 0 assignments, got %d", as.Len())
	}
}

// TestUpdateNonEmptyAssignmentsAppliedImmediately verifies real assignment
// updates are never delayed by the grace period logic.
func TestUpdateNonEmptyAssignmentsAppliedImmediately(t *testing.T) {
	as := &assignmentStore{assignments: map[string]string{}}

	modified, graceActive := as.Update([]ConfigurationAssignment{{BackendId: "backend-1", ConfigurationId: "config-1"}}, 3)

	if !modified {
		t.Fatalf("new assignment should report modified")
	}
	if graceActive {
		t.Fatalf("non-empty assignments should never report graceActive")
	}
	if as.Len() != 1 {
		t.Fatalf("want 1 assignment, got %d", as.Len())
	}
}

// TestWipePendingTracksGracePeriod verifies the flag the poll loop uses to
// keep re-evaluating a pending wipe across cycles the server reports as
// unmodified.
func TestWipePendingTracksGracePeriod(t *testing.T) {
	as := newStoreWithAssignment()
	threshold := 3

	if as.WipePending() {
		t.Fatalf("no wipe should be pending before any empty response")
	}

	as.Update([]ConfigurationAssignment{}, threshold) // streak = 1
	if !as.WipePending() {
		t.Fatalf("wipe should be pending during the grace period")
	}

	as.Update([]ConfigurationAssignment{}, threshold) // streak = 2
	if !as.WipePending() {
		t.Fatalf("wipe should still be pending at streak 2/%d", threshold)
	}

	as.Update([]ConfigurationAssignment{}, threshold) // streak = 3, wipe happens
	if as.WipePending() {
		t.Fatalf("no wipe should be pending once the wipe has been carried out")
	}
	if as.Len() != 0 {
		t.Fatalf("want 0 assignments after the confirmed wipe, got %d", as.Len())
	}
}

// TestWipePendingClearedByNonEmptyResponse verifies a real assignment arriving
// mid-grace-period releases the poll loop from forcing extra Update() calls.
func TestWipePendingClearedByNonEmptyResponse(t *testing.T) {
	as := newStoreWithAssignment()

	as.Update([]ConfigurationAssignment{}, 3)
	if !as.WipePending() {
		t.Fatalf("wipe should be pending during the grace period")
	}

	as.Update([]ConfigurationAssignment{{BackendId: "backend-1", ConfigurationId: "config-1"}}, 3)

	if as.WipePending() {
		t.Fatalf("a non-empty response should clear the pending wipe")
	}
}

// TestWipePendingUnsetWhenNothingToProtect verifies an already-empty store
// never asks the poll loop to keep spinning on empty responses.
func TestWipePendingUnsetWhenNothingToProtect(t *testing.T) {
	as := &assignmentStore{assignments: map[string]string{}}

	as.Update([]ConfigurationAssignment{}, 3)

	if as.WipePending() {
		t.Fatalf("an empty store has nothing to wipe, want WipePending false")
	}
}
