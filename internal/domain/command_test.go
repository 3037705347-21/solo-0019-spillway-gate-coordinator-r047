package domain

import (
	"errors"
	"testing"
	"time"
)

var testOrigin = time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)

func newTestCommand(t *testing.T, now time.Time, windowSeconds int) GateCommand {
	t.Helper()
	command, err := NewGateCommand(
		"cmd-review-1",
		"gate-west-001",
		2.0,
		1.0,
		3.0,
		"上游水位调整",
		"dispatcher-lin",
		windowSeconds,
		now,
	)
	if err != nil {
		t.Fatalf("NewGateCommand returned error: %v", err)
	}
	return command
}

func clearDecision(t *testing.T, now time.Time) InterlockDecision {
	t.Helper()
	decision, err := NewInterlockDecision("clear", "safety-wu", "联锁条件满足", now)
	if err != nil {
		t.Fatalf("NewInterlockDecision returned error: %v", err)
	}
	return decision
}

func blockedDecision(t *testing.T, now time.Time) InterlockDecision {
	t.Helper()
	decision, err := NewInterlockDecision("blocked", "safety-wu", "下游条件不满足", now)
	if err != nil {
		t.Fatalf("NewInterlockDecision returned error: %v", err)
	}
	return decision
}

func conflictCode(t *testing.T, err error) string {
	t.Helper()
	var domainErr *Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("expected domain error, got %T: %v", err, err)
	}
	if domainErr.Kind != KindConflict {
		t.Fatalf("expected conflict error, got kind %q (%v)", domainErr.Kind, err)
	}
	return domainErr.Code
}

func TestClearReviewWithinWindowApproves(t *testing.T) {
	created := testOrigin
	command := newTestCommand(t, created, 60)
	deadline := command.ReviewDeadline

	if err := command.Review(clearDecision(t, deadline.Add(-time.Nanosecond)), "token-1", deadline.Add(-time.Nanosecond)); err != nil {
		t.Fatalf("review before deadline returned error: %v", err)
	}
	if command.Status != CommandApproved {
		t.Fatalf("expected approved, got %q", command.Status)
	}
	if command.ExecutionToken != "token-1" {
		t.Fatalf("execution token not stored: %q", command.ExecutionToken)
	}
}

func TestReviewAtDeadlineExpiresPendingCommand(t *testing.T) {
	created := testOrigin
	command := newTestCommand(t, created, 60)
	deadline := command.ReviewDeadline

	err := command.Review(clearDecision(t, deadline), "token-1", deadline)
	if got := conflictCode(t, err); got != "command_review_expired" {
		t.Fatalf("expected command_review_expired, got %q", got)
	}
	if command.Status != CommandExpired {
		t.Fatalf("expected expired status, got %q", command.Status)
	}
	if command.ExecutionToken != "" {
		t.Fatalf("expired command must not carry a token: %q", command.ExecutionToken)
	}
	if command.ExpiredAt != deadline {
		t.Fatalf("expected expired at %v, got %v", deadline, command.ExpiredAt)
	}
	if command.Interlock != nil {
		t.Fatalf("expired review must not record an interlock decision")
	}
}

func TestReviewBeyondDeadlineHasNoGraceWindow(t *testing.T) {
	created := testOrigin
	command := newTestCommand(t, created, 60)
	deadline := command.ReviewDeadline

	// One second past the deadline previously fell inside the hard-coded grace
	// period and still allowed the gate to be reserved.
	now := deadline.Add(time.Second)
	authorization := command.ReviewAuthorization(now)
	if authorization.AllowsReview() {
		t.Fatal("review must not be allowed after the declared deadline")
	}
	if !authorization.RequiresExpiry() {
		t.Fatal("pending command past deadline must require expiry")
	}

	err := command.Review(clearDecision(t, now), "token-1", now)
	if got := conflictCode(t, err); got != "command_review_expired" {
		t.Fatalf("expected command_review_expired, got %q", got)
	}
	if command.Status != CommandExpired {
		t.Fatalf("expected expired status, got %q", command.Status)
	}
}

func TestBlockedReviewBeyondDeadlineExpiresInsteadOfRejecting(t *testing.T) {
	created := testOrigin
	command := newTestCommand(t, created, 60)
	deadline := command.ReviewDeadline

	now := deadline.Add(time.Minute)
	err := command.Review(blockedDecision(t, now), "", now)
	if got := conflictCode(t, err); got != "command_review_expired" {
		t.Fatalf("expected command_review_expired, got %q", got)
	}
	if command.Status != CommandExpired {
		t.Fatalf("expected expired status, got %q", command.Status)
	}
}

func TestApprovedAuthorizationNeverRequiresExpiry(t *testing.T) {
	created := testOrigin
	command := newTestCommand(t, created, 60)
	deadline := command.ReviewDeadline

	approvedAt := deadline.Add(-time.Minute)
	if err := command.Review(clearDecision(t, approvedAt), "token-1", approvedAt); err != nil {
		t.Fatalf("review returned error: %v", err)
	}

	for _, now := range []time.Time{deadline, deadline.Add(time.Hour), deadline.Add(24 * time.Hour)} {
		authorization := command.ReviewAuthorization(now)
		if authorization.State != ReviewAuthorizationApproved {
			t.Fatalf("at %v expected approved authorization, got %q", now, authorization.State)
		}
		if authorization.RequiresExpiry() {
			t.Fatalf("approved command at %v must never require expiry", now)
		}
		if command.ExpireIfReviewWindowElapsed(now) {
			t.Fatalf("approved command at %v must not be expired by the sweep", now)
		}
		if command.Status != CommandApproved || command.ExecutionToken != "token-1" {
			t.Fatalf("approval altered by expiry check at %v: status=%q token=%q", now, command.Status, command.ExecutionToken)
		}
	}
}

func TestApprovalStaysExecutableUntilExecutionEnds(t *testing.T) {
	created := testOrigin
	command := newTestCommand(t, created, 60)
	deadline := command.ReviewDeadline

	approvedAt := deadline.Add(-time.Minute)
	if err := command.Review(clearDecision(t, approvedAt), "token-1", approvedAt); err != nil {
		t.Fatalf("review returned error: %v", err)
	}

	beginAt := deadline.Add(time.Hour)
	if err := command.BeginExecution("token-1", "operator-chen", beginAt); err != nil {
		t.Fatalf("execution with the original token failed after deadline: %v", err)
	}
	if command.Status != CommandExecuting {
		t.Fatalf("expected executing, got %q", command.Status)
	}

	completeAt := deadline.Add(2 * time.Hour)
	if command.ExpireIfReviewWindowElapsed(completeAt) {
		t.Fatal("executing command must never be expired by the sweep")
	}
	if err := command.Complete(completeAt); err != nil {
		t.Fatalf("complete returned error: %v", err)
	}
	if command.Status != CommandCompleted || command.ExecutionToken != "" {
		t.Fatalf("unexpected terminal state: status=%q token=%q", command.Status, command.ExecutionToken)
	}
}

func TestBlockedReviewWithinWindowRejectsAndSweepSkips(t *testing.T) {
	created := testOrigin
	command := newTestCommand(t, created, 60)
	deadline := command.ReviewDeadline

	now := deadline.Add(-time.Second)
	if err := command.Review(blockedDecision(t, now), "", now); err != nil {
		t.Fatalf("blocked review returned error: %v", err)
	}
	if command.Status != CommandRejected {
		t.Fatalf("expected rejected, got %q", command.Status)
	}
	if command.ExpireIfReviewWindowElapsed(deadline.Add(time.Hour)) {
		t.Fatal("rejected command must never be expired by the sweep")
	}
	if command.Status != CommandRejected {
		t.Fatalf("rejected status changed: %q", command.Status)
	}
}
