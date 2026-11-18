package repository

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"taskflow/internal/domain"
)

func TestSQLiteRepositoryCRUDAndWorkflow(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test_taskflow.db")
	db, err := NewSQLiteDB(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteDB failed: %v", err)
	}
	defer db.Close()

	repo := NewTaskRepository(db)

	// Verify WAL mode is active
	var journalMode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode;").Scan(&journalMode); err != nil {
		t.Fatalf("failed to query journal_mode: %v", err)
	}
	if strings.ToLower(journalMode) != "wal" {
		t.Errorf("expected journal_mode=wal, got %q", journalMode)
	}

	// Create tasks
	t1 := &domain.Task{
		Title:         "Implement MCP Server",
		Description:   "Create stdio MCP server with 4 tools",
		DelegableToAI: true,
		Tags:          []string{"go", "mcp"},
	}
	id1, err := repo.Create(ctx, t1)
	if err != nil {
		t.Fatalf("Create t1 failed: %v", err)
	}

	t2 := &domain.Task{
		Title:         "Personal task not for AI",
		Description:   "Human only",
		DelegableToAI: false,
		Tags:          []string{"personal"},
	}
	id2, err := repo.Create(ctx, t2)
	if err != nil {
		t.Fatalf("Create t2 failed: %v", err)
	}

	// ListAvailableForAI with and without tagFilter
	avail, err := repo.ListAvailableForAI(ctx, "")
	if err != nil {
		t.Fatalf("ListAvailableForAI failed: %v", err)
	}
	if len(avail) != 1 || avail[0].ID != id1 {
		t.Fatalf("expected 1 AI-delegable task (id=%d), got %+v", id1, avail)
	}

	filtered, err := repo.ListAvailableForAI(ctx, "mcp")
	if err != nil {
		t.Fatalf("ListAvailableForAI(mcp) failed: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != id1 {
		t.Fatalf("expected 1 task matching tag 'mcp', got %+v", filtered)
	}

	filteredNone, err := repo.ListAvailableForAI(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("ListAvailableForAI(nonexistent) failed: %v", err)
	}
	if len(filteredNone) != 0 {
		t.Fatalf("expected 0 tasks matching 'nonexistent', got %+v", filteredNone)
	}

	// Claim non-delegable task must fail
	if err := repo.ClaimTask(ctx, id2, "agent-alpha"); !errors.Is(err, domain.ErrTaskNotClaimable) {
		t.Fatalf("expected ErrTaskNotClaimable for non-delegable task, got %v", err)
	}

	// Claim delegable task succeeds
	if err := repo.ClaimTask(ctx, id1, "agent-alpha"); err != nil {
		t.Fatalf("ClaimTask id1 failed: %v", err)
	}

	// SubmitTaskResult with requires_review = true -> moves to 'review'
	if err := repo.SubmitTaskResult(ctx, id1, "Completed MCP tools implementation", true); err != nil {
		t.Fatalf("SubmitTaskResult failed: %v", err)
	}

	got1, err := repo.GetByID(ctx, id1)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got1.Status != domain.StatusReview || got1.AssignedAgent != "agent-alpha" || got1.AINotes != "Completed MCP tools implementation" {
		t.Fatalf("unexpected task state after SubmitTaskResult: %+v", got1)
	}

	// ApproveTask moves from 'review' to 'done'
	if err := repo.ApproveTask(ctx, id1); err != nil {
		t.Fatalf("ApproveTask failed: %v", err)
	}
	got1, _ = repo.GetByID(ctx, id1)
	if got1.Status != domain.StatusDone {
		t.Fatalf("expected status 'done', got %q", got1.Status)
	}

	// ReturnToPending resets status to 'pending' and clears assigned_agent
	if err := repo.ReturnToPending(ctx, id1); err != nil {
		t.Fatalf("ReturnToPending failed: %v", err)
	}
	got1, _ = repo.GetByID(ctx, id1)
	if got1.Status != domain.StatusPending || got1.AssignedAgent != "" {
		t.Fatalf("expected status 'pending' and empty assigned_agent, got %+v", got1)
	}
}

// TestConcurrentClaimTaskCA1 validates Acceptance Criterion CA-1:
// When multiple separate SQLite connections/processes concurrently attempt to claim
// the same task_id, exactly one succeeds and the others receive ErrTaskNotClaimable
// without any SQLITE_BUSY errors.
func TestConcurrentClaimTaskCA1(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "concurrent_ca1.db")

	setupDB, err := NewSQLiteDB(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteDB setup failed: %v", err)
	}
	defer setupDB.Close()

	setupRepo := NewTaskRepository(setupDB)
	taskID, err := setupRepo.Create(ctx, &domain.Task{
		Title:         "Concurrent Claim Target",
		Description:   "CA-1 verification task",
		DelegableToAI: true,
		Tags:          []string{"concurrency", "ca1"},
	})
	if err != nil {
		t.Fatalf("Create task failed: %v", err)
	}

	const workers = 10
	repos := make([]*TaskRepository, workers)
	for i := 0; i < workers; i++ {
		conn, err := NewSQLiteDB(dbPath)
		if err != nil {
			t.Fatalf("NewSQLiteDB worker %d failed: %v", i, err)
		}
		defer conn.Close()
		repos[i] = NewTaskRepository(conn)
	}

	startBarrier := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-startBarrier
			agentName := fmt.Sprintf("agent-%d", idx)
			errs[idx] = repos[idx].ClaimTask(ctx, taskID, agentName)
		}(i)
	}

	close(startBarrier)
	wg.Wait()

	successCount := 0
	notClaimableCount := 0
	for i, err := range errs {
		switch {
		case err == nil:
			successCount++
		case errors.Is(err, domain.ErrTaskNotClaimable):
			notClaimableCount++
		default:
			t.Errorf("worker %d returned unexpected error (possible SQLITE_BUSY): %v", i, err)
		}
	}

	if successCount != 1 {
		t.Fatalf("CA-1 FAILED: expected exactly 1 successful claim, got %d", successCount)
	}
	if notClaimableCount != workers-1 {
		t.Fatalf("CA-1 FAILED: expected %d ErrTaskNotClaimable errors, got %d", workers-1, notClaimableCount)
	}

	claimedTask, err := setupRepo.GetByID(ctx, taskID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if claimedTask.Status != domain.StatusInProgress || claimedTask.AssignedAgent == "" {
		t.Fatalf("expected claimed task in_progress with assigned_agent, got %+v", claimedTask)
	}
}

func TestMoveToStatusSPEC002(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "move_status.db")
	db, err := NewSQLiteDB(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteDB failed: %v", err)
	}
	defer db.Close()

	repo := NewTaskRepository(db)
	id, err := repo.Create(ctx, &domain.Task{
		Title:         "Manual workflow task",
		DelegableToAI: false,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Move to in_progress as human
	if err := repo.MoveToStatus(ctx, id, domain.StatusInProgress, "humano"); err != nil {
		t.Fatalf("MoveToStatus in_progress failed: %v", err)
	}
	got, _ := repo.GetByID(ctx, id)
	if got.Status != domain.StatusInProgress || got.AssignedAgent != "humano" {
		t.Fatalf("expected in_progress with assigned_agent='humano', got %+v", got)
	}

	// Move to review preserves assigned_agent
	if err := repo.MoveToStatus(ctx, id, domain.StatusReview, "humano"); err != nil {
		t.Fatalf("MoveToStatus review failed: %v", err)
	}
	got, _ = repo.GetByID(ctx, id)
	if got.Status != domain.StatusReview || got.AssignedAgent != "humano" {
		t.Fatalf("expected review with assigned_agent='humano', got %+v", got)
	}

	// Move back to pending clears assigned_agent (CA-6)
	if err := repo.MoveToStatus(ctx, id, domain.StatusPending, ""); err != nil {
		t.Fatalf("MoveToStatus pending failed: %v", err)
	}
	got, _ = repo.GetByID(ctx, id)
	if got.Status != domain.StatusPending || got.AssignedAgent != "" {
		t.Fatalf("expected pending with empty assigned_agent, got %+v", got)
	}

	// Invalid status returns ErrValidation
	if err := repo.MoveToStatus(ctx, id, domain.Status("bogus"), ""); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected ErrValidation for invalid status, got %v", err)
	}
}
