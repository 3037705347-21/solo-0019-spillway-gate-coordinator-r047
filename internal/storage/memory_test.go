package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/spillway-gate-coordinator/internal/domain"
)

var repoTestOrigin = time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)

func newRepoFixture(t *testing.T, now time.Time, windowSeconds int) (*MemoryRepository, domain.Gate, domain.GateCommand) {
	t.Helper()
	repository := NewMemoryRepository()
	ctx := context.Background()

	gate, err := domain.NewGate("gate-east-01", "东侧泄洪闸", 5.0, 1.0, now)
	if err != nil {
		t.Fatalf("NewGate returned error: %v", err)
	}
	if err := repository.CreateGate(ctx, gate); err != nil {
		t.Fatalf("CreateGate returned error: %v", err)
	}

	command, err := domain.NewGateCommand(
		"cmd-expiry-01",
		gate.ID,
		2.5,
		1.0,
		5.0,
		"下游流量调整",
		"dispatcher-yang",
		windowSeconds,
		now,
	)
	if err != nil {
		t.Fatalf("NewGateCommand returned error: %v", err)
	}
	if err := repository.CreateCommand(ctx, command); err != nil {
		t.Fatalf("CreateCommand returned error: %v", err)
	}
	return repository, gate, command
}

func clearRepoDecision(now time.Time) domain.InterlockDecision {
	return domain.InterlockDecision{
		Verdict:    domain.InterlockClear,
		Verifier:   "safety-wu",
		Note:       "联锁条件满足",
		ReviewedAt: now.UTC(),
	}
}

func conflictCode(t *testing.T, err error) string {
	t.Helper()
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("expected domain error, got %T: %v", err, err)
	}
	if domainErr.Kind != domain.KindConflict {
		t.Fatalf("expected conflict, got kind %q (%v)", domainErr.Kind, err)
	}
	return domainErr.Code
}

func assertGate(t *testing.T, repository *MemoryRepository, gateID string, status domain.GateStatus, activeCommandID string) {
	t.Helper()
	gate, err := repository.Gate(context.Background(), gateID)
	if err != nil {
		t.Fatalf("Gate returned error: %v", err)
	}
	if gate.Status != status {
		t.Fatalf("gate status: expected %q, got %q", status, gate.Status)
	}
	if gate.ActiveCommandID != activeCommandID {
		t.Fatalf("active command: expected %q, got %q", activeCommandID, gate.ActiveCommandID)
	}
}

func assertCommand(t *testing.T, repository *MemoryRepository, commandID string, status domain.CommandStatus) domain.GateCommand {
	t.Helper()
	command, err := repository.Command(context.Background(), commandID)
	if err != nil {
		t.Fatalf("Command returned error: %v", err)
	}
	if command.Status != status {
		t.Fatalf("command status: expected %q, got %q", status, command.Status)
	}
	return command
}

func TestReviewPastDeadlineRejectedAndGateStaysIdle(t *testing.T) {
	created := repoTestOrigin
	repository, gate, command := newRepoFixture(t, created, 60)
	deadline := command.ReviewDeadline
	ctx := context.Background()

	now := deadline.Add(time.Second)
	_, _, err := repository.ReviewCommand(ctx, command.ID, clearRepoDecision(now), "token-late", now)
	if got := conflictCode(t, err); got != "command_review_expired" {
		t.Fatalf("expected command_review_expired, got %q", got)
	}

	assertCommand(t, repository, command.ID, domain.CommandExpired)
	assertGate(t, repository, gate.ID, domain.GateIdle, "")
}

func TestSweepExpiresPendingWithoutTouchingGate(t *testing.T) {
	created := repoTestOrigin
	repository, gate, command := newRepoFixture(t, created, 60)

	expired, err := repository.ExpireReviewWindows(context.Background(), command.ReviewDeadline.Add(time.Second))
	if err != nil {
		t.Fatalf("ExpireReviewWindows returned error: %v", err)
	}
	if len(expired) != 1 || expired[0] != command.ID {
		t.Fatalf("expected only %q to expire, got %v", command.ID, expired)
	}
	assertCommand(t, repository, command.ID, domain.CommandExpired)
	assertGate(t, repository, gate.ID, domain.GateIdle, "")
}

func TestSweepDoesNotExpireApprovedCommandOrReleaseGate(t *testing.T) {
	created := repoTestOrigin
	repository, gate, command := newRepoFixture(t, created, 60)
	deadline := command.ReviewDeadline
	ctx := context.Background()

	approvedAt := deadline.Add(-time.Minute)
	approved, reserved, err := repository.ReviewCommand(ctx, command.ID, clearRepoDecision(approvedAt), "token-keep", approvedAt)
	if err != nil {
		t.Fatalf("ReviewCommand returned error: %v", err)
	}
	if approved.Status != domain.CommandApproved || reserved.Status != domain.GateReserved {
		t.Fatalf("unexpected review result: command=%q gate=%q", approved.Status, reserved.Status)
	}

	// The background sweep runs long after the review window elapsed.
	expired, err := repository.ExpireReviewWindows(ctx, deadline.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("ExpireReviewWindows returned error: %v", err)
	}
	if len(expired) != 0 {
		t.Fatalf("no command may expire after approval, got %v", expired)
	}

	persisted := assertCommand(t, repository, command.ID, domain.CommandApproved)
	if persisted.ExecutionToken != "token-keep" {
		t.Fatalf("token invalidated by sweep: %q", persisted.ExecutionToken)
	}
	assertGate(t, repository, gate.ID, domain.GateReserved, command.ID)

	// The original credential must still execute the reserved gate.
	executed, settled, err := repository.ExecuteCommand(
		ctx, command.ID, "token-keep", "operator-chen", false, deadline.Add(25*time.Hour),
	)
	if err != nil {
		t.Fatalf("ExecuteCommand with original token failed: %v", err)
	}
	if executed.Status != domain.CommandCompleted {
		t.Fatalf("expected completed, got %q", executed.Status)
	}
	if settled.Status != domain.GateIdle || settled.ActiveCommandID != "" || settled.Aperture != command.TargetAperture {
		t.Fatalf("gate not settled correctly: %+v", settled)
	}
}

func TestExecuteAfterApprovalSurvivesInterleavedSweep(t *testing.T) {
	created := repoTestOrigin
	repository, gate, command := newRepoFixture(t, created, 60)
	deadline := command.ReviewDeadline
	ctx := context.Background()

	approvedAt := deadline.Add(-time.Minute)
	if _, _, err := repository.ReviewCommand(ctx, command.ID, clearRepoDecision(approvedAt), "token-run", approvedAt); err != nil {
		t.Fatalf("ReviewCommand returned error: %v", err)
	}

	// Execution and the sweep happen at the same instant, long after the window.
	now := deadline.Add(time.Hour)
	expired, err := repository.ExpireReviewWindows(ctx, now)
	if err != nil {
		t.Fatalf("ExpireReviewWindows returned error: %v", err)
	}
	if len(expired) != 0 {
		t.Fatalf("approved command must not be swept, got %v", expired)
	}
	executed, settled, err := repository.ExecuteCommand(ctx, command.ID, "token-run", "operator-chen", true, now)
	if err != nil {
		t.Fatalf("fault-path execution failed: %v", err)
	}
	if executed.Status != domain.CommandAborted {
		t.Fatalf("expected aborted, got %q", executed.Status)
	}
	if settled.Status != domain.GateFaulted || settled.ActiveCommandID != "" || settled.Aperture != command.TargetAperture {
		t.Fatalf("gate fault latch not preserved: %+v", settled)
	}

	// A later sweep must not resurrect or rewrite the latched state.
	if _, err := repository.ExpireReviewWindows(ctx, now.Add(time.Hour)); err != nil {
		t.Fatalf("second sweep returned error: %v", err)
	}
	assertCommand(t, repository, command.ID, domain.CommandAborted)
	assertGate(t, repository, gate.ID, domain.GateFaulted, "")
}

func TestReviewRacingSweepSweepFirstLeavesExpiredAndIdle(t *testing.T) {
	created := repoTestOrigin
	repository, gate, command := newRepoFixture(t, created, 60)
	deadline := command.ReviewDeadline
	ctx := context.Background()

	// Linearization 1: sweep commits before the review at the same instant.
	sweepAt := deadline.Add(time.Second)
	reviewAt := sweepAt
	if _, err := repository.ExpireReviewWindows(ctx, sweepAt); err != nil {
		t.Fatalf("ExpireReviewWindows returned error: %v", err)
	}
	_, _, err := repository.ReviewCommand(ctx, command.ID, clearRepoDecision(reviewAt), "token-a", reviewAt)
	if got := conflictCode(t, err); got != "command_review_expired" {
		t.Fatalf("expected command_review_expired after expiry, got %q", got)
	}

	assertCommand(t, repository, command.ID, domain.CommandExpired)
	assertGate(t, repository, gate.ID, domain.GateIdle, "")
}

func TestReviewRacingSweepReviewFirstKeepsReservation(t *testing.T) {
	created := repoTestOrigin
	repository, gate, command := newRepoFixture(t, created, 60)
	deadline := command.ReviewDeadline
	ctx := context.Background()

	// Linearization 2: review commits one instant before the deadline; sweep after.
	reviewAt := deadline.Add(-time.Nanosecond)
	approved, _, err := repository.ReviewCommand(ctx, command.ID, clearRepoDecision(reviewAt), "token-b", reviewAt)
	if err != nil {
		t.Fatalf("on-time ReviewCommand returned error: %v", err)
	}
	if approved.Status != domain.CommandApproved {
		t.Fatalf("expected approved, got %q", approved.Status)
	}

	expired, err := repository.ExpireReviewWindows(ctx, deadline.Add(time.Hour))
	if err != nil {
		t.Fatalf("ExpireReviewWindows returned error: %v", err)
	}
	if len(expired) != 0 {
		t.Fatalf("sweep must not expire approved command, got %v", expired)
	}

	assertCommand(t, repository, command.ID, domain.CommandApproved)
	assertGate(t, repository, gate.ID, domain.GateReserved, command.ID)
}

func TestExpiredCommandTokenNeverIssuedAndReuseRejected(t *testing.T) {
	created := repoTestOrigin
	repository, gate, command := newRepoFixture(t, created, 60)
	deadline := command.ReviewDeadline
	ctx := context.Background()

	if _, err := repository.ExpireReviewWindows(ctx, deadline.Add(time.Second)); err != nil {
		t.Fatalf("ExpireReviewWindows returned error: %v", err)
	}
	_, _, err := repository.ExecuteCommand(
		ctx, command.ID, "any-token", "operator-chen", false, deadline.Add(time.Hour),
	)
	if got := conflictCode(t, err); got != "command_not_executable" {
		t.Fatalf("expected command_not_executable, got %q", got)
	}
	assertGate(t, repository, gate.ID, domain.GateIdle, "")
}
