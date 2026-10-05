package storage

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"example.com/spillway-gate-coordinator/internal/domain"
)

// TestConcurrentReviewSweepExecuteConsistency hammers one gate with many
// pending commands while reviews, expiry sweeps and executions interleave.
// Exactly one command may ever occupy the gate, and its state must always
// match the gate's real occupation.
func TestConcurrentReviewSweepExecuteConsistency(t *testing.T) {
	created := repoTestOrigin
	repository := NewMemoryRepository()
	ctx := context.Background()

	gate, err := domain.NewGate("gate-race-01", "并发泄洪闸", 10.0, 1.0, created)
	if err != nil {
		t.Fatalf("NewGate returned error: %v", err)
	}
	if err := repository.CreateGate(ctx, gate); err != nil {
		t.Fatalf("CreateGate returned error: %v", err)
	}

	const commandCount = 40
	ids := make([]string, 0, commandCount)
	tokens := make(map[string]string)
	var tokensMu sync.Mutex
	deadline := created.Add(200 * time.Millisecond)

	for i := 0; i < commandCount; i++ {
		command, err := domain.NewGateCommand(
			fmt.Sprintf("cmd-race-%02d", i),
			gate.ID,
			2.0+float64(i%5),
			1.0,
			10.0,
			"并发水位调整",
			"dispatcher-lin",
			200, // 200ms review window
			created,
		)
		if err != nil {
			t.Fatalf("NewGateCommand returned error: %v", err)
		}
		if err := repository.CreateCommand(ctx, command); err != nil {
			t.Fatalf("CreateCommand returned error: %v", err)
		}
		ids = append(ids, command.ID)
	}

	decision := domain.InterlockDecision{
		Verdict:    domain.InterlockClear,
		Verifier:   "safety-wu",
		Note:       "并发复核",
		ReviewedAt: deadline,
	}

	// Reviews spread across the deadline: half effectively on time, half late.
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, id := range ids {
		wg.Add(1)
		go func(index int, commandID string) {
			defer wg.Done()
			<-start
			reviewAt := deadline.Add(time.Duration(index-commandCount/2) * time.Millisecond)
			token := fmt.Sprintf("token-%s", commandID)
			_, reservedGate, reviewErr := repository.ReviewCommand(ctx, commandID, decision, token, reviewAt)
			if reviewErr == nil && reservedGate.Status == domain.GateReserved && reservedGate.ActiveCommandID == commandID {
				tokensMu.Lock()
				tokens[commandID] = token
				tokensMu.Unlock()
			}
		}(i, id)
	}

	// Background sweeps run continuously through the same window.
	stopSweeps := make(chan struct{})
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(offset time.Duration) {
			defer wg.Done()
			<-start
			for {
				select {
				case <-stopSweeps:
					return
				default:
				}
				repository.ExpireReviewWindows(ctx, deadline.Add(offset*time.Millisecond))
				offset++
			}
		}(time.Duration(i))
	}

	// Executors repeatedly try every issued token; an approval that reserved
	// the gate must complete exactly once.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for attempt := 0; attempt < commandCount*2; attempt++ {
				tokensMu.Lock()
				for commandID, token := range tokens {
					_, _, _ = repository.ExecuteCommand(
						ctx, commandID, token, "operator-chen", false, deadline.Add(time.Hour),
					)
				}
				tokensMu.Unlock()
			}
		}()
	}

	close(start)
	time.Sleep(300 * time.Millisecond)
	close(stopSweeps)
	wg.Wait()

	// Final reconciliation: count the commands per state and verify the gate
	// is occupied by exactly the single consistent active command, if any.
	statusCount := map[domain.CommandStatus]int{}
	var occupiedBy string
	for _, id := range ids {
		command, err := repository.Command(ctx, id)
		if err != nil {
			t.Fatalf("Command %s returned error: %v", id, err)
		}
		statusCount[command.Status]++
		switch command.Status {
		case domain.CommandApproved, domain.CommandExecuting:
			if occupiedBy != "" {
				t.Fatalf("two commands still occupy the gate: %q and %q", occupiedBy, id)
			}
			occupiedBy = id
			if command.ExecutionToken == "" {
				t.Fatalf("active command %q lost its token", id)
			}
		case domain.CommandCompleted:
			if command.ExecutionToken != "" {
				t.Fatalf("completed command %q kept a reusable token", id)
			}
		case domain.CommandExpired:
			if command.ExecutionToken != "" {
				t.Fatalf("expired command %q carries a token: gate occupation lost", id)
			}
		}
	}

	finalGate, err := repository.Gate(ctx, gate.ID)
	if err != nil {
		t.Fatalf("Gate returned error: %v", err)
	}
	if occupiedBy != "" {
		if finalGate.Status != domain.GateReserved || finalGate.ActiveCommandID != occupiedBy {
			t.Fatalf("gate occupation mismatch: gate=%q/%q active command=%q",
				finalGate.Status, finalGate.ActiveCommandID, occupiedBy)
		}
	} else {
		if finalGate.Status == domain.GateReserved || finalGate.Status == domain.GateMoving {
			t.Fatalf("gate %q references %q without an active command",
				finalGate.Status, finalGate.ActiveCommandID)
		}
		if finalGate.ActiveCommandID != "" {
			t.Fatalf("idle/final gate still references %q", finalGate.ActiveCommandID)
		}
	}

	// Commands may complete serially (the gate frees after each), but every
	// transition must be consistent with the gate's real occupation above.
	t.Logf("final distribution: %v, gate=%q, active=%q", statusCount, finalGate.Status, finalGate.ActiveCommandID)
}
