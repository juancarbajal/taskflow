package mcp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"taskflow/internal/domain"
	"taskflow/internal/repository"
)

func TestMCPServerResourcesAndToolsCA2(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "mcp_test.db")
	db, err := repository.NewSQLiteDB(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteDB failed: %v", err)
	}
	defer db.Close()

	repo := repository.NewTaskRepository(db)
	srv := NewServer(repo)

	// 1. Test create_subtask tool
	createReq := mcp.CallToolRequest{}
	createReq.Params.Name = "create_subtask"
	createReq.Params.Arguments = map[string]any{
		"title":           "Write unit tests for MCP",
		"description":     "Verify CA-2 compliance",
		"delegable_to_ai": true,
		"tags":            []any{"go", "mcp"},
	}
	createRes, err := srv.handleCreateSubtask(ctx, createReq)
	if err != nil || createRes.IsError {
		t.Fatalf("handleCreateSubtask failed: err=%v, res=%+v", err, createRes)
	}

	// 2. Test resource tasks://pending-for-ai
	resContents, err := srv.handleReadPendingResource(ctx, mcp.ReadResourceRequest{})
	if err != nil || len(resContents) != 1 {
		t.Fatalf("handleReadPendingResource failed: err=%v, len=%d", err, len(resContents))
	}
	textRes, ok := resContents[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("expected TextResourceContents, got %T", resContents[0])
	}
	var pendingTasks []domain.Task
	if err := json.Unmarshal([]byte(textRes.Text), &pendingTasks); err != nil {
		t.Fatalf("unmarshal resource JSON failed: %v", err)
	}
	if len(pendingTasks) != 1 || pendingTasks[0].Title != "Write unit tests for MCP" {
		t.Fatalf("unexpected pending tasks in resource: %+v", pendingTasks)
	}
	taskID := pendingTasks[0].ID

	// 3. Test list_available_tasks tool with tag_filter
	listReq := mcp.CallToolRequest{}
	listReq.Params.Name = "list_available_tasks"
	listReq.Params.Arguments = map[string]any{"tag_filter": "mcp"}
	listRes, err := srv.handleListAvailableTasks(ctx, listReq)
	if err != nil || listRes.IsError {
		t.Fatalf("handleListAvailableTasks failed: err=%v, res=%+v", err, listRes)
	}
	listText := mcp.GetTextFromContent(listRes.Content[0])
	var listedDTOs []AvailableTaskDTO
	if err := json.Unmarshal([]byte(listText), &listedDTOs); err != nil {
		t.Fatalf("unmarshal list_available_tasks output failed: %v", err)
	}
	if len(listedDTOs) != 1 || listedDTOs[0].ID != taskID {
		t.Fatalf("expected 1 listed task with ID=%d, got %+v", taskID, listedDTOs)
	}

	// 4. Test claim_task tool (first claim succeeds, second claim returns isError: true)
	claimReq1 := mcp.CallToolRequest{}
	claimReq1.Params.Name = "claim_task"
	claimReq1.Params.Arguments = map[string]any{
		"task_id":    float64(taskID),
		"agent_name": "agent-1",
	}
	claimRes1, err := srv.handleClaimTask(ctx, claimReq1)
	if err != nil || claimRes1.IsError {
		t.Fatalf("first claim_task should succeed: err=%v, res=%+v", err, claimRes1)
	}

	claimReq2 := mcp.CallToolRequest{}
	claimReq2.Params.Name = "claim_task"
	claimReq2.Params.Arguments = map[string]any{
		"task_id":    float64(taskID),
		"agent_name": "agent-2",
	}
	claimRes2, err := srv.handleClaimTask(ctx, claimReq2)
	if err != nil {
		t.Fatalf("unexpected error from second handleClaimTask: %v", err)
	}
	if !claimRes2.IsError {
		t.Fatalf("expected second claim_task on already claimed task to return isError=true")
	}

	// 5. Test submit_task_result tool (requires_review = true -> status = 'review')
	submitReq := mcp.CallToolRequest{}
	submitReq.Params.Name = "submit_task_result"
	submitReq.Params.Arguments = map[string]any{
		"task_id":         float64(taskID),
		"ai_notes":        "Implemented and verified all 4 tools",
		"requires_review": true,
	}
	submitRes, err := srv.handleSubmitTaskResult(ctx, submitReq)
	if err != nil || submitRes.IsError {
		t.Fatalf("handleSubmitTaskResult failed: err=%v, res=%+v", err, submitRes)
	}

	updatedTask, err := repo.GetByID(ctx, taskID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updatedTask.Status != domain.StatusReview || updatedTask.AINotes != "Implemented and verified all 4 tools" {
		t.Fatalf("unexpected task state after submit_task_result: %+v", updatedTask)
	}
}
