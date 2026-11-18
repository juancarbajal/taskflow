package tui

import (
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"taskflow/internal/domain"
	"taskflow/internal/repository"
)

// ViewMode represents the active overlay/screen state in the TUI.
type ViewMode int

const (
	ModeBoard ViewMode = iota
	ModeDetail
	ModeForm
	ModeConfirmDelete
)

// ActionID identifies a clickable quick-action button in the header/toolbar.
type ActionID string

const (
	ActionNewTask       ActionID = "new_task"
	ActionToggleAI      ActionID = "toggle_ai"
	ActionInProgress    ActionID = "in_progress"
	ActionApprove       ActionID = "approve_done"
	ActionReturnPending ActionID = "return_pending"
)

// Rect represents a 2D terminal cell bounding box [X, X+W) x [Y, Y+H).
type Rect struct {
	X int
	Y int
	W int
	H int
}

// Contains returns true if cell coordinate (x, y) is inside the rectangle.
func (r Rect) Contains(x, y int) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// CardHitbox maps a rendered task card on screen to its column, row index, and TaskID.
type CardHitbox struct {
	ColumnIdx int
	TaskIdx   int
	TaskID    int64
	Bounds    Rect
}

// ColumnOrder defines the 4 Kanban columns required by SPEC-001 §4.1.1.
var ColumnOrder = [4]domain.Status{
	domain.StatusPending,
	domain.StatusInProgress,
	domain.StatusReview,
	domain.StatusDone,
}

// ColumnTitles provides display headers for each Kanban column.
var ColumnTitles = [4]string{
	"PENDING",
	"IN PROGRESS",
	"REVIEW",
	"DONE",
}

const (
	headerLines      = 3
	footerLines      = 2
	cardHeight       = 4
	doubleClickLimit = 400 * time.Millisecond
	pollInterval     = 2 * time.Second
)

// Model holds the state of the Bubble Tea Kanban application.
type Model struct {
	repo *repository.TaskRepository

	width  int
	height int

	mode ViewMode

	// Tasks grouped into the 4 Kanban columns.
	columns     [4][]domain.Task
	activeCol   int
	selectedRow [4]int
	scrollOff   [4]int

	// Hitboxes for mouse interaction (CA-3).
	buttonRects map[ActionID]Rect
	columnRects [4]Rect
	cardRects   []CardHitbox

	// Double-click tracking.
	lastClickTaskID int64
	lastClickTime   time.Time

	// Detail modal scroll offset.
	detailTask   *domain.Task
	detailScroll int

	// Form modal state (Create / Edit task).
	isEditForm     bool
	editingTaskID  int64
	formFocusIdx   int // 0=Title, 1=Description, 2=Tags, 3=DelegableToAI, 4=Status(edit), 5=Save, 6=Cancel
	titleInput     textinput.Model
	descInput      textarea.Model
	tagsInput      textinput.Model
	delegableToAI  bool
	formStatus     domain.Status
	formStatusRect Rect
	formSaveRect   Rect
	formCloseRect  Rect
	formCheckRect  Rect

	// Status message / error notification.
	statusMsg string
	errMsg    string
}

// NewModel initializes the Bubble Tea TUI model backed by the SQLite TaskRepository.
func NewModel(repo *repository.TaskRepository) Model {
	ti := textinput.New()
	ti.Placeholder = "Título de la tarea..."
	ti.CharLimit = 120
	ti.Width = 48

	ta := textarea.New()
	ta.Placeholder = "Descripción detallada..."
	ta.SetWidth(48)
	ta.SetHeight(4)

	tgi := textinput.New()
	tgi.Placeholder = "go, mcp, docs (separados por coma)"
	tgi.CharLimit = 120
	tgi.Width = 48

	m := Model{
		repo:          repo,
		width:         100,
		height:        30,
		mode:          ModeBoard,
		buttonRects:   make(map[ActionID]Rect),
		titleInput:    ti,
		descInput:     ta,
		tagsInput:     tgi,
		delegableToAI: true,
		statusMsg:     "Listo. Usa ratón o teclado (n: nueva, e: editar, a: aprobar, d: borrar, r: recargar, q: salir).",
	}
	m.recalculateLayout()
	return m
}

// SelectedTask returns a pointer to the currently focused task on the board, or nil.
func (m *Model) SelectedTask() *domain.Task {
	colTasks := m.columns[m.activeCol]
	if len(colTasks) == 0 {
		return nil
	}
	idx := m.selectedRow[m.activeCol]
	if idx < 0 || idx >= len(colTasks) {
		return nil
	}
	t := colTasks[idx]
	return &t
}

func (m *Model) visibleCardsPerColumn() int {
	boardHeight := m.height - headerLines - footerLines - 4
	if boardHeight < cardHeight {
		return 1
	}
	count := boardHeight / cardHeight
	if count < 1 {
		return 1
	}
	return count
}

func (m *Model) recalculateLayout() {
	if m.width <= 0 {
		m.width = 100
	}
	if m.height <= 0 {
		m.height = 30
	}

	// Quick action buttons in row Y = 1 (second line of header).
	// Labels:
	// [+ Nueva Tarea]  [Toggle IA]  [En Proceso]  [Aprobar / Mover a Done]  [Devolver a Pending]
	m.buttonRects = make(map[ActionID]Rect, 5)
	btnSpecs := []struct {
		id    ActionID
		label string
	}{
		{ActionNewTask, "[+ Nueva Tarea]"},
		{ActionToggleAI, "[Toggle IA]"},
		{ActionInProgress, "[En Proceso]"},
		{ActionApprove, "[Aprobar / Mover a Done]"},
		{ActionReturnPending, "[Devolver a Pending]"},
	}

	curX := 1
	for _, b := range btnSpecs {
		w := len([]rune(b.label)) + 2
		m.buttonRects[b.id] = Rect{
			X: curX,
			Y: 1,
			W: w,
			H: 1,
		}
		curX += w + 1
	}

	// 4 Kanban columns starting at Y = headerLines.
	colWidth := m.width / 4
	if colWidth < 16 {
		colWidth = 16
	}
	boardY := headerLines
	boardH := m.height - headerLines - footerLines
	if boardH < 6 {
		boardH = 6
	}

	m.cardRects = m.cardRects[:0]
	maxCards := m.visibleCardsPerColumn()

	for col := 0; col < 4; col++ {
		colX := col * colWidth
		w := colWidth
		if col == 3 {
			w = m.width - colX
		}
		m.columnRects[col] = Rect{
			X: colX,
			Y: boardY,
			W: w,
			H: boardH,
		}

		tasks := m.columns[col]
		startIdx := m.scrollOff[col]
		if startIdx < 0 {
			startIdx = 0
		}
		endIdx := startIdx + maxCards
		if endIdx > len(tasks) {
			endIdx = len(tasks)
		}

		cardStartY := boardY + 2 // below column title header
		for i := startIdx; i < endIdx; i++ {
			slot := i - startIdx
			cy := cardStartY + slot*cardHeight
			m.cardRects = append(m.cardRects, CardHitbox{
				ColumnIdx: col,
				TaskIdx:   i,
				TaskID:    tasks[i].ID,
				Bounds: Rect{
					X: colX + 1,
					Y: cy,
					W: w - 2,
					H: cardHeight,
				},
			})
		}
	}
}

func (m *Model) setTasks(tasks []domain.Task) {
	var prevSelectedID int64
	if sel := m.SelectedTask(); sel != nil {
		prevSelectedID = sel.ID
	}

	var grouped [4][]domain.Task
	for _, t := range tasks {
		switch t.Status {
		case domain.StatusPending:
			grouped[0] = append(grouped[0], t)
		case domain.StatusInProgress:
			grouped[1] = append(grouped[1], t)
		case domain.StatusReview:
			grouped[2] = append(grouped[2], t)
		case domain.StatusDone:
			grouped[3] = append(grouped[3], t)
		default:
			grouped[0] = append(grouped[0], t)
		}
	}
	m.columns = grouped

	// Preserve selection across auto-refresh or status movements when possible.
	if prevSelectedID != 0 {
		foundInActive := false
		for i, t := range m.columns[m.activeCol] {
			if t.ID == prevSelectedID {
				m.selectedRow[m.activeCol] = i
				foundInActive = true
				break
			}
		}
		if !foundInActive {
			for col := 0; col < 4; col++ {
				for i, t := range m.columns[col] {
					if t.ID == prevSelectedID {
						m.selectedRow[col] = i
						break
					}
				}
			}
		}
	}

	for col := 0; col < 4; col++ {
		m.clampColumnSelection(col)
	}
	if m.detailTask != nil {
		for _, t := range tasks {
			if t.ID == m.detailTask.ID {
				copyTask := t
				m.detailTask = &copyTask
				break
			}
		}
	}
	m.recalculateLayout()
}

func (m *Model) clampColumnSelection(col int) {
	n := len(m.columns[col])
	if n == 0 {
		m.selectedRow[col] = 0
		m.scrollOff[col] = 0
		return
	}
	if m.selectedRow[col] >= n {
		m.selectedRow[col] = n - 1
	}
	if m.selectedRow[col] < 0 {
		m.selectedRow[col] = 0
	}

	maxVis := m.visibleCardsPerColumn()
	if m.selectedRow[col] < m.scrollOff[col] {
		m.scrollOff[col] = m.selectedRow[col]
	} else if m.selectedRow[col] >= m.scrollOff[col]+maxVis {
		m.scrollOff[col] = m.selectedRow[col] - maxVis + 1
	}
	if m.scrollOff[col] < 0 {
		m.scrollOff[col] = 0
	}
}
