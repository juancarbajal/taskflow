package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"taskflow/internal/domain"
	"taskflow/internal/repository"
)

type tickMsg time.Time

type tasksLoadedMsg struct {
	tasks  []domain.Task
	status string
	err    error
}

type actionResultMsg struct {
	status string
	err    error
}

func tickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func loadTasksCmd(repo *repository.TaskRepository, statusText string) tea.Cmd {
	return func() tea.Msg {
		tasks, err := repo.ListAll(context.Background())
		return tasksLoadedMsg{
			tasks:  tasks,
			status: statusText,
			err:    err,
		}
	}
}

// Init loads the initial task list from SQLite and schedules the 2-second auto-refresh tick (CA-4).
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		loadTasksCmd(m.repo, ""),
		tickCmd(),
	)
}

// Update processes keyboard, mouse (tea.MouseMsg), window resize, and periodic tick events.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalculateLayout()
		return m, nil

	case tickMsg:
		return m, tea.Batch(
			loadTasksCmd(m.repo, ""),
			tickCmd(),
		)

	case tasksLoadedMsg:
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			return m, nil
		}
		m.errMsg = ""
		if msg.status != "" {
			m.statusMsg = msg.status
		}
		m.setTasks(msg.tasks)
		return m, nil

	case actionResultMsg:
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			return m, nil
		}
		m.errMsg = ""
		return m, loadTasksCmd(m.repo, msg.status)

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if m.mode == ModeForm {
		cmd := m.updateFormInputs(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	switch m.mode {
	case ModeDetail:
		switch msg.String() {
		case "esc", "q", "enter":
			m.mode = ModeBoard
			m.detailTask = nil
			m.detailScroll = 0
		case "up", "k":
			if m.detailScroll > 0 {
				m.detailScroll--
			}
		case "down", "j":
			m.detailScroll++
		case "a":
			if m.detailTask != nil {
				id := m.detailTask.ID
				m.mode = ModeBoard
				m.detailTask = nil
				return m, m.approveTaskCmd(id)
			}
		}
		return m, nil

	case ModeConfirmDelete:
		switch strings.ToLower(msg.String()) {
		case "y", "s", "enter":
			sel := m.SelectedTask()
			m.mode = ModeBoard
			if sel != nil {
				return m, m.deleteTaskCmd(sel.ID)
			}
		case "n", "esc", "q":
			m.mode = ModeBoard
			m.statusMsg = "Eliminación cancelada."
		}
		return m, nil

	case ModeForm:
		switch msg.String() {
		case "esc":
			m.mode = ModeBoard
			m.statusMsg = "Formulario cerrado."
			return m, nil
		case "tab", "down":
			if m.formFocusIdx == 1 && msg.String() == "down" {
				break
			}
			m.advanceFormFocus(1)
			return m, nil
		case "shift+tab", "up":
			if m.formFocusIdx == 1 && msg.String() == "up" {
				break
			}
			m.advanceFormFocus(-1)
			return m, nil
		case "left":
			if m.formFocusIdx == 4 && m.isEditForm {
				m.cycleFormStatus(-1)
				return m, nil
			}
		case "right":
			if m.formFocusIdx == 4 && m.isEditForm {
				m.cycleFormStatus(1)
				return m, nil
			}
		case " ":
			if m.formFocusIdx == 3 {
				m.delegableToAI = !m.delegableToAI
				return m, nil
			}
			if m.formFocusIdx == 4 && m.isEditForm {
				m.cycleFormStatus(1)
				return m, nil
			}
		case "enter":
			switch m.formFocusIdx {
			case 3:
				m.delegableToAI = !m.delegableToAI
				return m, nil
			case 4:
				if m.isEditForm {
					m.cycleFormStatus(1)
					return m, nil
				}
			case 5, 0, 2:
				return m.submitForm()
			case 6:
				m.mode = ModeBoard
				m.statusMsg = "Formulario cancelado."
				return m, nil
			}
		case "ctrl+s":
			return m.submitForm()
		}
		cmd := m.updateFormInputs(msg)
		return m, cmd
	}

	// ModeBoard keyboard shortcuts (SPEC-001 §4.1.3 & SPEC-002 §2.3)
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "n":
		m.openCreateForm()
		return m, nil
	case "e":
		if sel := m.SelectedTask(); sel != nil {
			m.openEditForm(sel)
		}
		return m, nil
	case "a":
		if sel := m.SelectedTask(); sel != nil {
			return m, m.approveTaskCmd(sel.ID)
		}
		return m, nil
	case "t":
		if sel := m.SelectedTask(); sel != nil {
			return m, m.toggleAICmd(sel.ID)
		}
		return m, nil
	case "p":
		if sel := m.SelectedTask(); sel != nil {
			return m, m.returnToPendingCmd(sel.ID)
		}
		return m, nil
	case "]", "shift+right":
		if sel := m.SelectedTask(); sel != nil && m.activeCol < 3 {
			nextCol := m.activeCol + 1
			m.activeCol = nextCol
			return m, m.moveTaskStatusCmd(sel.ID, ColumnOrder[nextCol], "humano")
		}
		return m, nil
	case "[", "shift+left":
		if sel := m.SelectedTask(); sel != nil && m.activeCol > 0 {
			prevCol := m.activeCol - 1
			m.activeCol = prevCol
			return m, m.moveTaskStatusCmd(sel.ID, ColumnOrder[prevCol], "humano")
		}
		return m, nil
	case "1":
		if sel := m.SelectedTask(); sel != nil {
			m.activeCol = 0
			return m, m.moveTaskStatusCmd(sel.ID, domain.StatusPending, "")
		}
		return m, nil
	case "2":
		if sel := m.SelectedTask(); sel != nil {
			m.activeCol = 1
			return m, m.moveTaskStatusCmd(sel.ID, domain.StatusInProgress, "humano")
		}
		return m, nil
	case "3":
		if sel := m.SelectedTask(); sel != nil {
			m.activeCol = 2
			return m, m.moveTaskStatusCmd(sel.ID, domain.StatusReview, "")
		}
		return m, nil
	case "4":
		if sel := m.SelectedTask(); sel != nil {
			m.activeCol = 3
			return m, m.moveTaskStatusCmd(sel.ID, domain.StatusDone, "")
		}
		return m, nil
	case "d":
		if m.SelectedTask() != nil {
			m.mode = ModeConfirmDelete
		}
		return m, nil
	case "r":
		return m, loadTasksCmd(m.repo, "Tablero sincronizado desde SQLite.")
	case "enter":
		if sel := m.SelectedTask(); sel != nil {
			copyTask := *sel
			m.detailTask = &copyTask
			m.detailScroll = 0
			m.mode = ModeDetail
		}
		return m, nil
	case "left", "h":
		if m.activeCol > 0 {
			m.activeCol--
			m.clampColumnSelection(m.activeCol)
			m.recalculateLayout()
		}
		return m, nil
	case "right", "l":
		if m.activeCol < 3 {
			m.activeCol++
			m.clampColumnSelection(m.activeCol)
			m.recalculateLayout()
		}
		return m, nil
	case "up", "k":
		if m.selectedRow[m.activeCol] > 0 {
			m.selectedRow[m.activeCol]--
			m.clampColumnSelection(m.activeCol)
			m.recalculateLayout()
		}
		return m, nil
	case "down", "j":
		if m.selectedRow[m.activeCol] < len(m.columns[m.activeCol])-1 {
			m.selectedRow[m.activeCol]++
			m.clampColumnSelection(m.activeCol)
			m.recalculateLayout()
		}
		return m, nil
	}

	return m, nil
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Wheel scrolling (SPEC-001 §4.1.2, CA-3)
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		delta := 1
		if msg.Button == tea.MouseButtonWheelUp {
			delta = -1
		}

		if m.mode == ModeDetail {
			m.detailScroll += delta
			if m.detailScroll < 0 {
				m.detailScroll = 0
			}
			return m, nil
		}

		if m.mode == ModeBoard {
			targetCol := m.activeCol
			for col := 0; col < 4; col++ {
				if m.columnRects[col].Contains(msg.X, msg.Y) {
					targetCol = col
					break
				}
			}
			m.activeCol = targetCol
			n := len(m.columns[targetCol])
			if n > 0 {
				m.selectedRow[targetCol] += delta
				m.clampColumnSelection(targetCol)
				m.recalculateLayout()
			}
			return m, nil
		}
		return m, nil
	}

	// Left-click press interactions (SPEC-001 §4.1.2, CA-3)
	isLeftPress := msg.Button == tea.MouseButtonLeft && (msg.Action == tea.MouseActionPress || msg.Type == tea.MouseLeft)
	if !isLeftPress {
		return m, nil
	}

	if m.mode == ModeDetail {
		m.mode = ModeBoard
		m.detailTask = nil
		m.detailScroll = 0
		return m, nil
	}

	if m.mode == ModeConfirmDelete {
		m.mode = ModeBoard
		return m, nil
	}

	if m.mode == ModeForm {
		if m.formCheckRect.Contains(msg.X, msg.Y) {
			m.delegableToAI = !m.delegableToAI
			return m, nil
		}
		if m.isEditForm && m.formStatusRect.Contains(msg.X, msg.Y) {
			m.cycleFormStatus(1)
			return m, nil
		}
		if m.formSaveRect.Contains(msg.X, msg.Y) {
			return m.submitForm()
		}
		if m.formCloseRect.Contains(msg.X, msg.Y) {
			m.mode = ModeBoard
			m.statusMsg = "Formulario cancelado."
			return m, nil
		}
		return m, nil
	}

	// 1. Check quick action buttons in header toolbar
	for action, rect := range m.buttonRects {
		if rect.Contains(msg.X, msg.Y) {
			return m.executeQuickAction(action)
		}
	}

	// 2. Check task card hitboxes (single click selects, double click opens detail modal)
	now := time.Now()
	for _, card := range m.cardRects {
		if card.Bounds.Contains(msg.X, msg.Y) {
			m.activeCol = card.ColumnIdx
			m.selectedRow[card.ColumnIdx] = card.TaskIdx
			m.clampColumnSelection(card.ColumnIdx)
			m.recalculateLayout()

			if m.lastClickTaskID == card.TaskID && !m.lastClickTime.IsZero() && now.Sub(m.lastClickTime) <= doubleClickLimit {
				if sel := m.SelectedTask(); sel != nil {
					copyTask := *sel
					m.detailTask = &copyTask
					m.detailScroll = 0
					m.mode = ModeDetail
				}
				m.lastClickTaskID = 0
				m.lastClickTime = time.Time{}
				return m, nil
			}

			m.lastClickTaskID = card.TaskID
			m.lastClickTime = now
			return m, nil
		}
	}

	// 3. Check column background hitboxes
	for col := 0; col < 4; col++ {
		if m.columnRects[col].Contains(msg.X, msg.Y) {
			m.activeCol = col
			m.clampColumnSelection(col)
			m.recalculateLayout()
			return m, nil
		}
	}

	return m, nil
}

func (m Model) executeQuickAction(action ActionID) (tea.Model, tea.Cmd) {
	switch action {
	case ActionNewTask:
		m.openCreateForm()
		return m, nil
	case ActionToggleAI:
		if sel := m.SelectedTask(); sel != nil {
			return m, m.toggleAICmd(sel.ID)
		}
		m.statusMsg = "Selecciona una tarea primero."
		return m, nil
	case ActionInProgress:
		if sel := m.SelectedTask(); sel != nil {
			m.activeCol = 1
			return m, m.moveTaskStatusCmd(sel.ID, domain.StatusInProgress, "humano")
		}
		m.statusMsg = "Selecciona una tarea para mover a En Proceso."
		return m, nil
	case ActionApprove:
		if sel := m.SelectedTask(); sel != nil {
			return m, m.approveTaskCmd(sel.ID)
		}
		m.statusMsg = "Selecciona una tarea para aprobar."
		return m, nil
	case ActionReturnPending:
		if sel := m.SelectedTask(); sel != nil {
			return m, m.returnToPendingCmd(sel.ID)
		}
		m.statusMsg = "Selecciona una tarea para devolver a Pending."
		return m, nil
	}
	return m, nil
}

func (m Model) submitForm() (tea.Model, tea.Cmd) {
	title := strings.TrimSpace(m.titleInput.Value())
	if title == "" {
		m.errMsg = "El título de la tarea es obligatorio."
		return m, nil
	}
	desc := strings.TrimSpace(m.descInput.Value())
	tags := m.parseFormTags()
	delegable := m.delegableToAI
	chosenStatus := m.formStatus
	isEdit := m.isEditForm
	editID := m.editingTaskID

	m.mode = ModeBoard
	m.errMsg = ""

	if isEdit {
		existing := m.SelectedTask()
		status := chosenStatus
		if !status.IsValid() {
			status = domain.StatusPending
		}
		assigned := ""
		notes := ""
		if existing != nil && existing.ID == editID {
			assigned = existing.AssignedAgent
			notes = existing.AINotes
		}
		if status == domain.StatusPending {
			assigned = ""
		} else if status == domain.StatusInProgress && assigned == "" {
			assigned = "humano"
		}
		return m, func() tea.Msg {
			t := &domain.Task{
				ID:            editID,
				Title:         title,
				Description:   desc,
				Status:        status,
				DelegableToAI: delegable,
				Tags:          tags,
				AssignedAgent: assigned,
				AINotes:       notes,
			}
			err := m.repo.Update(context.Background(), t)
			return actionResultMsg{
				status: fmt.Sprintf("Tarea #%d actualizada (%s).", editID, status),
				err:    err,
			}
		}
	}

	return m, func() tea.Msg {
		t := &domain.Task{
			Title:         title,
			Description:   desc,
			Status:        domain.StatusPending,
			DelegableToAI: delegable,
			Tags:          tags,
		}
		id, err := m.repo.Create(context.Background(), t)
		return actionResultMsg{
			status: fmt.Sprintf("Tarea #%d creada.", id),
			err:    err,
		}
	}
}

func (m Model) toggleAICmd(taskID int64) tea.Cmd {
	return func() tea.Msg {
		err := m.repo.ToggleDelegableToAI(context.Background(), taskID)
		return actionResultMsg{
			status: fmt.Sprintf("Tarea #%d: estado Delegable a IA alternado.", taskID),
			err:    err,
		}
	}
}

func (m Model) approveTaskCmd(taskID int64) tea.Cmd {
	return func() tea.Msg {
		err := m.repo.ApproveTask(context.Background(), taskID)
		return actionResultMsg{
			status: fmt.Sprintf("Tarea #%d aprobada y movida a DONE.", taskID),
			err:    err,
		}
	}
}

func (m Model) returnToPendingCmd(taskID int64) tea.Cmd {
	return func() tea.Msg {
		err := m.repo.ReturnToPending(context.Background(), taskID)
		return actionResultMsg{
			status: fmt.Sprintf("Tarea #%d devuelta a PENDING (agente limpiado).", taskID),
			err:    err,
		}
	}
}

func (m Model) moveTaskStatusCmd(taskID int64, status domain.Status, actor string) tea.Cmd {
	return func() tea.Msg {
		err := m.repo.MoveToStatus(context.Background(), taskID, status, actor)
		return actionResultMsg{
			status: fmt.Sprintf("Tarea #%d movida a %s.", taskID, strings.ToUpper(string(status))),
			err:    err,
		}
	}
}

func (m Model) deleteTaskCmd(taskID int64) tea.Cmd {
	return func() tea.Msg {
		err := m.repo.Delete(context.Background(), taskID)
		return actionResultMsg{
			status: fmt.Sprintf("Tarea #%d eliminada.", taskID),
			err:    err,
		}
	}
}
