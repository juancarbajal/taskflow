package tui

import (
	"context"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"taskflow/internal/domain"
	"taskflow/internal/repository"
)

func TestMouseInteractionsAndLiveSyncCA3AndCA4(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tui_test.db")
	db, err := repository.NewSQLiteDB(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteDB failed: %v", err)
	}
	defer db.Close()

	repo := repository.NewTaskRepository(db)

	// Insert multiple tasks in PENDING (for scroll test) and one task in REVIEW (for mouse approve test)
	for i := 1; i <= 6; i++ {
		_, err := repo.Create(ctx, &domain.Task{
			Title:         "Pending Task",
			Description:   "Scroll test card",
			Status:        domain.StatusPending,
			DelegableToAI: true,
			Tags:          []string{"tui"},
		})
		if err != nil {
			t.Fatalf("Create pending task %d failed: %v", i, err)
		}
	}

	reviewTask := &domain.Task{
		Title:         "AI Completed Task Waiting Review",
		Description:   "Needs human click on Approve button",
		Status:        domain.StatusReview,
		DelegableToAI: true,
		AssignedAgent: "claude-code",
		AINotes:       "All tests passing",
		Tags:          []string{"review"},
	}
	reviewID, err := repo.Create(ctx, reviewTask)
	if err != nil {
		t.Fatalf("Create reviewTask failed: %v", err)
	}

	m := NewModel(repo)
	updatedModel, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 28})
	m = updatedModel.(Model)

	// Load initial tasks synchronously via loadTasksCmd
	loadMsg := loadTasksCmd(repo, "")()
	updatedModel, _ = m.Update(loadMsg)
	m = updatedModel.(Model)

	if len(m.columns[0]) != 6 {
		t.Fatalf("expected 6 pending tasks in col 0, got %d", len(m.columns[0]))
	}
	if len(m.columns[2]) != 1 {
		t.Fatalf("expected 1 review task in col 2, got %d", len(m.columns[2]))
	}

	// CA-3 Part 1: Mouse wheel down over PENDING column scrolls selection
	col0Rect := m.columnRects[0]
	wheelMsg := tea.MouseMsg{
		X:      col0Rect.X + 2,
		Y:      col0Rect.Y + 3,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	}
	updatedModel, _ = m.Update(wheelMsg)
	m = updatedModel.(Model)
	if m.selectedRow[0] != 1 {
		t.Fatalf("CA-3 wheel scroll failed: expected selectedRow[0]=1, got %d", m.selectedRow[0])
	}

	// CA-3 Part 2: Left-click on the REVIEW task card selects column 2 and focuses the review task
	var reviewCardHitbox *CardHitbox
	for i := range m.cardRects {
		if m.cardRects[i].TaskID == reviewID {
			reviewCardHitbox = &m.cardRects[i]
			break
		}
	}
	if reviewCardHitbox == nil {
		t.Fatalf("expected hitbox for review task #%d", reviewID)
	}

	clickCardMsg := tea.MouseMsg{
		X:      reviewCardHitbox.Bounds.X + 1,
		Y:      reviewCardHitbox.Bounds.Y + 1,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}
	updatedModel, _ = m.Update(clickCardMsg)
	m = updatedModel.(Model)

	if m.activeCol != 2 {
		t.Fatalf("CA-3 left-click card failed: expected activeCol=2 (REVIEW), got %d", m.activeCol)
	}
	sel := m.SelectedTask()
	if sel == nil || sel.ID != reviewID {
		t.Fatalf("CA-3 left-click card failed: expected selected task ID=%d, got %+v", reviewID, sel)
	}

	// CA-3 Part 3: Left-click on the "[Aprobar / Mover a Done]" action button approves the task without keyboard
	approveBtnRect, ok := m.buttonRects[ActionApprove]
	if !ok {
		t.Fatalf("expected ActionApprove button hitbox to be registered")
	}
	clickApproveMsg := tea.MouseMsg{
		X:      approveBtnRect.X + 2,
		Y:      approveBtnRect.Y,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}
	updatedModel, cmd := m.Update(clickApproveMsg)
	m = updatedModel.(Model)
	if cmd == nil {
		t.Fatalf("expected non-nil command after clicking Approve button")
	}

	// Execute the approve command and feed its result back into Update
	resMsg := cmd()
	updatedModel, reloadCmd := m.Update(resMsg)
	m = updatedModel.(Model)
	if reloadCmd == nil {
		t.Fatalf("expected reload command after approve actionResultMsg")
	}
	updatedModel, _ = m.Update(reloadCmd())
	m = updatedModel.(Model)

	if len(m.columns[2]) != 0 || len(m.columns[3]) != 1 || m.columns[3][0].ID != reviewID {
		t.Fatalf("CA-3 approve via mouse click failed: expected task #%d in DONE (col 3), got col2=%d col3=%d",
			reviewID, len(m.columns[2]), len(m.columns[3]))
	}

	// CA-4: External agent claims a pending task in SQLite; 2s tick automatically moves it to IN PROGRESS
	pendingID := m.columns[0][0].ID
	if err := repo.ClaimTask(ctx, pendingID, "external-mcp-agent"); err != nil {
		t.Fatalf("external ClaimTask failed: %v", err)
	}

	updatedModel, tickBatchCmd := m.Update(tickMsg{})
	m = updatedModel.(Model)
	if tickBatchCmd == nil {
		t.Fatalf("expected batch command from tickMsg")
	}

	// Simulate the loadTasksCmd triggered by tickMsg
	updatedModel, _ = m.Update(loadTasksCmd(repo, "")())
	m = updatedModel.(Model)

	if len(m.columns[1]) != 1 || m.columns[1][0].ID != pendingID || m.columns[1][0].AssignedAgent != "external-mcp-agent" {
		t.Fatalf("CA-4 live sync failed: expected task #%d in IN PROGRESS with agent 'external-mcp-agent', got %+v",
			pendingID, m.columns[1])
	}
}

func TestManualStatusTransitionsSPEC002CA5AndCA6(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tui_spec002.db")
	db, err := repository.NewSQLiteDB(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteDB failed: %v", err)
	}
	defer db.Close()

	repo := repository.NewTaskRepository(db)
	taskID, err := repo.Create(ctx, &domain.Task{
		Title:         "Human Task",
		Description:   "Move manually across columns",
		Status:        domain.StatusPending,
		DelegableToAI: false,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	m := NewModel(repo)
	updatedModel, _ := m.Update(tea.WindowSizeMsg{Width: 130, Height: 30})
	m = updatedModel.(Model)
	updatedModel, _ = m.Update(loadTasksCmd(repo, "")())
	m = updatedModel.(Model)

	// CA-5 Part 1: Click [En Proceso] button moves task from PENDING to IN PROGRESS
	inProgBtn, ok := m.buttonRects[ActionInProgress]
	if !ok {
		t.Fatalf("expected ActionInProgress button hitbox to be registered")
	}
	updatedModel, cmd := m.Update(tea.MouseMsg{
		X:      inProgBtn.X + 2,
		Y:      inProgBtn.Y,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	m = updatedModel.(Model)
	if cmd == nil {
		t.Fatalf("expected command from clicking [En Proceso]")
	}
	updatedModel, reloadCmd := m.Update(cmd())
	m = updatedModel.(Model)
	updatedModel, _ = m.Update(reloadCmd())
	m = updatedModel.(Model)

	if len(m.columns[1]) != 1 || m.columns[1][0].ID != taskID || m.columns[1][0].AssignedAgent != "humano" {
		t.Fatalf("CA-5 button failed: expected task #%d in IN PROGRESS with assigned_agent='humano', got %+v", taskID, m.columns[1])
	}

	// CA-5 Part 2: Keyboard ']' moves task from IN PROGRESS to REVIEW
	updatedModel, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = updatedModel.(Model)
	updatedModel, reloadCmd = m.Update(cmd())
	m = updatedModel.(Model)
	updatedModel, _ = m.Update(reloadCmd())
	m = updatedModel.(Model)

	if len(m.columns[2]) != 1 || m.columns[2][0].ID != taskID {
		t.Fatalf("CA-5 ']' shortcut failed: expected task #%d in REVIEW, got %+v", taskID, m.columns[2])
	}

	// CA-6: Keyboard '[' moves task back towards PENDING, and '1' moves directly to PENDING clearing assigned_agent
	updatedModel, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = updatedModel.(Model)
	updatedModel, reloadCmd = m.Update(cmd())
	m = updatedModel.(Model)
	updatedModel, _ = m.Update(reloadCmd())
	m = updatedModel.(Model)

	if len(m.columns[0]) != 1 || m.columns[0][0].ID != taskID || m.columns[0][0].AssignedAgent != "" {
		t.Fatalf("CA-6 '1' shortcut failed: expected task #%d in PENDING with empty assigned_agent, got %+v", taskID, m.columns[0])
	}

	// CA-5 Part 3: Edit form ('e') allows changing status to IN PROGRESS
	updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m = updatedModel.(Model)
	if m.mode != ModeForm || !m.isEditForm {
		t.Fatalf("expected ModeForm in edit mode")
	}
	m.formFocusIdx = 4
	updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updatedModel.(Model)
	if m.formStatus != domain.StatusInProgress {
		t.Fatalf("expected formStatus=%q, got %q", domain.StatusInProgress, m.formStatus)
	}
	updatedModel, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updatedModel.(Model)
	updatedModel, reloadCmd = m.Update(cmd())
	m = updatedModel.(Model)
	updatedModel, _ = m.Update(reloadCmd())
	m = updatedModel.(Model)

	if len(m.columns[1]) != 1 || m.columns[1][0].ID != taskID {
		t.Fatalf("CA-5 edit form status failed: expected task #%d in IN PROGRESS, got %+v", taskID, m.columns[1])
	}
}
