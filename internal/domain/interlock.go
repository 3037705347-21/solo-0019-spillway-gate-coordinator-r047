package domain

import (
	"strings"
	"time"
)

type InterlockVerdict string

const (
	InterlockClear   InterlockVerdict = "clear"
	InterlockBlocked InterlockVerdict = "blocked"
)

type InterlockDecision struct {
	Verdict    InterlockVerdict
	Verifier   string
	Note       string
	ReviewedAt time.Time
}

func NewInterlockDecision(verdict, verifier, note string, now time.Time) (InterlockDecision, error) {
	normalized := InterlockVerdict(strings.ToLower(strings.TrimSpace(verdict)))
	if normalized != InterlockClear && normalized != InterlockBlocked {
		return InterlockDecision{}, Invalid("invalid_interlock_verdict", "verdict must be clear or blocked")
	}

	verifier = strings.TrimSpace(verifier)
	if verifier == "" {
		return InterlockDecision{}, Invalid("invalid_interlock_verifier", "verifier is required")
	}

	note = strings.TrimSpace(note)
	if normalized == InterlockBlocked && note == "" {
		return InterlockDecision{}, Invalid("invalid_interlock_note", "blocked decisions require a note")
	}

	return InterlockDecision{
		Verdict:    normalized,
		Verifier:   verifier,
		Note:       note,
		ReviewedAt: now.UTC(),
	}, nil
}

func (d InterlockDecision) IsClear() bool {
	return d.Verdict == InterlockClear
}
