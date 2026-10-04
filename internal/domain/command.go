package domain

import (
	"strings"
	"time"
)

type CommandStatus string

const (
	CommandPending   CommandStatus = "pending"
	CommandApproved  CommandStatus = "approved"
	CommandRejected  CommandStatus = "rejected"
	CommandExpired   CommandStatus = "expired"
	CommandExecuting CommandStatus = "executing"
	CommandCompleted CommandStatus = "completed"
	CommandAborted   CommandStatus = "aborted"
)

const (
	defaultReviewWindow = 15 * time.Minute
	maxReviewWindow     = 24 * time.Hour
	reviewGracePeriod   = 2 * time.Second
)

type ReviewAuthorizationState string

const (
	ReviewAuthorizationPending  ReviewAuthorizationState = "pending"
	ReviewAuthorizationApproved ReviewAuthorizationState = "approved"
	ReviewAuthorizationElapsed  ReviewAuthorizationState = "elapsed"
	ReviewAuthorizationSettled  ReviewAuthorizationState = "settled"
)

type ReviewAuthorization struct {
	State         ReviewAuthorizationState
	Deadline      time.Time
	GraceDeadline time.Time
	EvaluatedAt   time.Time
	Reason        string
}

type GateCommand struct {
	ID               string
	GateID           string
	TargetAperture   float64
	StartingAperture float64
	Reason           string
	RequestedBy      string
	Status           CommandStatus
	Interlock        *InterlockDecision
	ExecutionToken   string
	ExecutedBy       string
	TerminalReason   string
	ReviewDeadline   time.Time
	ExpiredAt        time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Version          int64
}

func NewGateCommand(
	commandID string,
	gateID string,
	targetAperture float64,
	startingAperture float64,
	maxAperture float64,
	reason string,
	requestedBy string,
	reviewWindowSeconds int,
	now time.Time,
) (GateCommand, error) {
	commandID = strings.TrimSpace(commandID)
	if commandID == "" {
		return GateCommand{}, Invalid("invalid_command_id", "command id is required")
	}

	gateID = strings.TrimSpace(gateID)
	if gateID == "" {
		return GateCommand{}, Invalid("invalid_gate_reference", "gate id is required")
	}

	reason = strings.TrimSpace(reason)
	if reason == "" {
		return GateCommand{}, Invalid("invalid_command_reason", "reason is required")
	}

	requestedBy = strings.TrimSpace(requestedBy)
	if requestedBy == "" {
		return GateCommand{}, Invalid("invalid_requested_by", "requested_by is required")
	}

	if startingAperture < 0 || startingAperture > maxAperture {
		return GateCommand{}, Invalid("invalid_starting_aperture", "starting aperture is outside the gate limit")
	}
	if targetAperture < 0 || targetAperture > maxAperture {
		return GateCommand{}, Invalid("invalid_target_aperture", "target aperture is outside the gate limit")
	}
	if targetAperture == startingAperture {
		return GateCommand{}, Invalid("no_aperture_change", "target aperture must differ from the current aperture")
	}

	reviewWindow, err := reviewWindowDuration(reviewWindowSeconds)
	if err != nil {
		return GateCommand{}, err
	}
	createdAt := now.UTC()

	return GateCommand{
		ID:               commandID,
		GateID:           gateID,
		TargetAperture:   targetAperture,
		StartingAperture: startingAperture,
		Reason:           reason,
		RequestedBy:      requestedBy,
		Status:           CommandPending,
		ReviewDeadline:   createdAt.Add(reviewWindow),
		CreatedAt:        createdAt,
		UpdatedAt:        createdAt,
		Version:          1,
	}, nil
}

func reviewWindowDuration(seconds int) (time.Duration, error) {
	if seconds == 0 {
		return defaultReviewWindow, nil
	}
	if seconds < 1 || seconds > int(maxReviewWindow/time.Second) {
		return 0, Invalid(
			"invalid_review_window",
			"review window must be between one second and twenty-four hours",
		)
	}
	return time.Duration(seconds) * time.Second, nil
}

func (c GateCommand) ReviewAuthorization(now time.Time) ReviewAuthorization {
	evaluatedAt := now.UTC()
	deadline := c.ReviewDeadline
	if deadline.IsZero() {
		deadline = evaluatedAt
	}
	state := ReviewAuthorizationSettled
	switch c.Status {
	case CommandPending:
		state = ReviewAuthorizationPending
	case CommandApproved:
		state = ReviewAuthorizationApproved
	case CommandExpired:
		state = ReviewAuthorizationElapsed
	}
	return ReviewAuthorization{
		State:         state,
		Deadline:      deadline,
		GraceDeadline: deadline.Add(reviewGracePeriod),
		EvaluatedAt:   evaluatedAt,
		Reason:        "review_window_elapsed",
	}
}

func (a ReviewAuthorization) AllowsReview() bool {
	return a.State == ReviewAuthorizationPending &&
		a.EvaluatedAt.Before(a.GraceDeadline)
}

func (a ReviewAuthorization) RequiresExpiry() bool {
	return !a.EvaluatedAt.Before(a.GraceDeadline)
}

func (a ReviewAuthorization) Error() error {
	if a.RequiresExpiry() {
		return Conflict("command_review_expired", "review window has elapsed")
	}
	return Conflict("command_not_pending", "only pending commands can be reviewed")
}

func (a ReviewAuthorization) ExpireCommand(command *GateCommand) bool {
	if command.Status != CommandPending && command.Status != CommandApproved {
		return false
	}
	command.ExecutionToken = ""
	command.Status = CommandExpired
	command.TerminalReason = a.Reason
	command.ExpiredAt = a.EvaluatedAt
	command.UpdatedAt = a.EvaluatedAt
	command.Version++
	return true
}

func (c *GateCommand) ExpireIfReviewWindowElapsed(now time.Time) bool {
	authorization := c.ReviewAuthorization(now)
	if !authorization.RequiresExpiry() {
		return false
	}
	return authorization.ExpireCommand(c)
}

func (c *GateCommand) Review(decision InterlockDecision, token string, now time.Time) error {
	if c.Status != CommandPending {
		return Conflict("command_not_pending", "only pending commands can be reviewed")
	}
	if c.ExpireIfReviewWindowElapsed(now) {
		return Conflict("command_review_expired", "review window has elapsed")
	}

	decisionCopy := decision
	c.Interlock = &decisionCopy
	c.UpdatedAt = now.UTC()
	c.Version++

	if decision.IsClear() {
		token = strings.TrimSpace(token)
		if token == "" {
			return Invalid("missing_execution_token", "clear decisions require an execution token")
		}
		c.ExecutionToken = token
		c.Status = CommandApproved
		return nil
	}

	c.ExecutionToken = ""
	c.Status = CommandRejected
	return nil
}

func (c *GateCommand) BeginExecution(token, operator string, now time.Time) error {
	if c.Status != CommandApproved {
		return Conflict("command_not_executable", "only approved commands can execute")
	}
	if strings.TrimSpace(operator) == "" {
		return Invalid("invalid_operator", "operator is required")
	}
	if strings.TrimSpace(token) == "" || token != c.ExecutionToken {
		return Conflict("invalid_execution_token", "execution token is invalid")
	}

	c.ExecutedBy = strings.TrimSpace(operator)
	c.Status = CommandExecuting
	c.UpdatedAt = now.UTC()
	c.Version++
	return nil
}

func (c *GateCommand) Complete(now time.Time) error {
	if c.Status != CommandExecuting {
		return Conflict("command_not_executing", "command is not executing")
	}

	c.ExecutionToken = ""
	c.Status = CommandCompleted
	c.UpdatedAt = now.UTC()
	c.Version++
	return nil
}

func (c *GateCommand) Abort(reason string, now time.Time) error {
	if c.Status != CommandExecuting {
		return Conflict("command_not_executing", "command is not executing")
	}

	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "gate_execution_failed"
	}
	c.ExecutionToken = ""
	c.TerminalReason = reason
	c.Status = CommandAborted
	c.UpdatedAt = now.UTC()
	c.Version++
	return nil
}
