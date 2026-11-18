package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"taskflow/internal/domain"
)

var (
	titleBarStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#F8FAFC")).
			Background(lipgloss.Color("#1E293B")).
			Padding(0, 1)

	buttonStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#0F172A")).
			Background(lipgloss.Color("#38BDF8")).
			Padding(0, 1)

	approveButtonStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#0F172A")).
				Background(lipgloss.Color("#4ADE80")).
				Padding(0, 1)

	colHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#E2E8F0")).
			Background(lipgloss.Color("#334155")).
			Padding(0, 1)

	activeColHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#0F172A")).
				Background(lipgloss.Color("#38BDF8")).
				Padding(0, 1)

	cardBorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#475569")).
			Padding(0, 1)

	selectedCardBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.ThickBorder()).
				BorderForeground(lipgloss.Color("#38BDF8")).
				Padding(0, 1)

	aiBadgeStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#0F172A")).
			Background(lipgloss.Color("#A78BFA")).
			Padding(0, 1)

	agentBadgeStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#0F172A")).
			Background(lipgloss.Color("#FBBF24")).
			Padding(0, 1)

	modalBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("#38BDF8")).
			Padding(1, 2)

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#94A3B8"))

	errStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#F87171"))
)

// View renders the entire TUI state (Kanban board, action bar, and modals).
func (m Model) View() string {
	switch m.mode {
	case ModeDetail:
		return m.renderDetailModal()
	case ModeForm:
		return m.renderFormModal()
	case ModeConfirmDelete:
		return m.renderConfirmDeleteModal()
	default:
		return m.renderBoard()
	}
}

func (m Model) renderBoard() string {
	var out strings.Builder

	// Line 0: Application Title Bar
	totalTasks := len(m.columns[0]) + len(m.columns[1]) + len(m.columns[2]) + len(m.columns[3])
	headerText := fmt.Sprintf("TASKFLOW — Kanban & MCP Server | Total: %d tareas | Auto-sync: 2s", totalTasks)
	out.WriteString(titleBarStyle.Width(m.width).Render(headerText))
	out.WriteString("\n")

	// Line 1: Quick Action Buttons (aligned with recalculateLayout hitboxes)
	btnNew := buttonStyle.Render("[+ Nueva Tarea]")
	btnToggle := buttonStyle.Render("[Toggle IA]")
	btnInProgress := buttonStyle.Render("[En Proceso]")
	btnApprove := approveButtonStyle.Render("[Aprobar / Mover a Done]")
	btnReturn := buttonStyle.Render("[Devolver a Pending]")
	out.WriteString(" " + strings.Join([]string{btnNew, btnToggle, btnInProgress, btnApprove, btnReturn}, " "))
	out.WriteString("\n")

	// Line 2: Separator
	out.WriteString(strings.Repeat("─", maxInt(m.width, 40)))
	out.WriteString("\n")

	// Kanban Columns
	colViews := make([]string, 4)
	maxCards := m.visibleCardsPerColumn()

	for col := 0; col < 4; col++ {
		colW := m.columnRects[col].W
		if colW < 18 {
			colW = 18
		}
		innerW := colW - 4
		if innerW < 10 {
			innerW = 10
		}

		tasks := m.columns[col]
		hdrLabel := fmt.Sprintf("%s (%d)", ColumnTitles[col], len(tasks))
		var hdr string
		if col == m.activeCol {
			hdr = activeColHeaderStyle.Width(colW - 1).Render(hdrLabel)
		} else {
			hdr = colHeaderStyle.Width(colW - 1).Render(hdrLabel)
		}

		var colBuilder strings.Builder
		colBuilder.WriteString(hdr + "\n")

		if len(tasks) == 0 {
			emptyStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#64748B")).
				Width(colW - 1).
				Padding(1, 1)
			colBuilder.WriteString(emptyStyle.Render("(Sin tareas)"))
		} else {
			start := m.scrollOff[col]
			if start < 0 {
				start = 0
			}
			end := start + maxCards
			if end > len(tasks) {
				end = len(tasks)
			}
			for i := start; i < end; i++ {
				task := tasks[i]
				isSelected := col == m.activeCol && i == m.selectedRow[col]
				colBuilder.WriteString(m.renderTaskCard(task, innerW, isSelected))
				colBuilder.WriteString("\n")
			}
		}

		colBox := lipgloss.NewStyle().Width(colW).Render(colBuilder.String())
		colViews[col] = colBox
	}

	board := lipgloss.JoinHorizontal(lipgloss.Top, colViews[0], colViews[1], colViews[2], colViews[3])
	out.WriteString(board)
	out.WriteString("\n")

	// Footer status & keyboard shortcuts
	if m.errMsg != "" {
		out.WriteString(errStyle.Render("Error: "+m.errMsg) + "\n")
	} else {
		out.WriteString(footerStyle.Render(m.statusMsg) + "\n")
	}
	helpLine := "Atajos: [n] Nueva | [e] Editar | [[ / ]] Mover Columna | [1-4] Estado | [a] Aprobar | [t] Toggle IA | [p] Pending | [d] Borrar | [q] Salir"
	out.WriteString(footerStyle.Render(truncateText(helpLine, m.width)))

	return out.String()
}

func (m Model) renderTaskCard(task domain.Task, innerW int, isSelected bool) string {
	line1 := fmt.Sprintf("#%d %s", task.ID, task.Title)
	line1 = truncateText(line1, innerW)

	var badges []string
	if task.DelegableToAI {
		badges = append(badges, aiBadgeStyle.Render("IA"))
	} else {
		badges = append(badges, "[Humano]")
	}
	if task.AssignedAgent != "" {
		badges = append(badges, agentBadgeStyle.Render("@"+truncateText(task.AssignedAgent, 14)))
	}
	if len(task.Tags) > 0 {
		badges = append(badges, "["+truncateText(strings.Join(task.Tags, ","), 14)+"]")
	}
	line2 := strings.Join(badges, " ")

	content := line1 + "\n" + line2
	if isSelected {
		return selectedCardBorderStyle.Width(innerW).Render(content)
	}
	return cardBorderStyle.Width(innerW).Render(content)
}

func (m Model) renderDetailModal() string {
	if m.detailTask == nil {
		return m.renderBoard()
	}
	t := m.detailTask
	delegableStr := "Sí (Delegable a Agentes MCP)"
	if !t.DelegableToAI {
		delegableStr = "No (Solo Humano)"
	}
	agentStr := t.AssignedAgent
	if agentStr == "" {
		agentStr = "(Ninguno)"
	}
	tagsStr := strings.Join(t.Tags, ", ")
	if tagsStr == "" {
		tagsStr = "(Sin etiquetas)"
	}
	desc := t.Description
	if desc == "" {
		desc = "(Sin descripción)"
	}
	notes := t.AINotes
	if notes == "" {
		notes = "(Sin notas de agente IA)"
	}

	bodyLines := []string{
		fmt.Sprintf("TAREA #%d: %s", t.ID, t.Title),
		strings.Repeat("─", 56),
		fmt.Sprintf("Estado:          %s", strings.ToUpper(string(t.Status))),
		fmt.Sprintf("Delegable a IA:  %s", delegableStr),
		fmt.Sprintf("Agente Asignado: %s", agentStr),
		fmt.Sprintf("Tags:            %s", tagsStr),
		"",
		"Descripción:",
		desc,
		"",
		"Notas del Agente IA (ai_notes):",
		notes,
		"",
		"[a] Aprobar / Mover a Done   [Esc / Enter / Clic] Cerrar",
	}

	fullText := strings.Join(bodyLines, "\n")
	splitLines := strings.Split(fullText, "\n")
	start := m.detailScroll
	if start >= len(splitLines) {
		start = maxInt(0, len(splitLines)-1)
	}
	visible := strings.Join(splitLines[start:], "\n")

	modalW := minInt(m.width-6, 72)
	if modalW < 40 {
		modalW = 40
	}
	return "\n" + modalBoxStyle.Width(modalW).Render(visible)
}

func (m Model) renderFormModal() string {
	title := "Crear Nueva Tarea"
	if m.isEditForm {
		title = fmt.Sprintf("Editar Tarea #%d", m.editingTaskID)
	}

	checkMark := "[ ]"
	if m.delegableToAI {
		checkMark = "[x]"
	}
	checkFocus := ""
	if m.formFocusIdx == 3 {
		checkFocus = " <"
	}

	saveBtn := buttonStyle.Render("[ Guardar (Enter) ]")
	cancelBtn := colHeaderStyle.Render("[ Cancelar (Esc) ]")
	if m.formFocusIdx == 5 {
		saveBtn = approveButtonStyle.Render("[ *Guardar (Enter)* ]")
	}
	if m.formFocusIdx == 6 {
		cancelBtn = activeColHeaderStyle.Render("[ *Cancelar (Esc)* ]")
	}

	lines := []string{
		lipgloss.NewStyle().Bold(true).Render(title),
		strings.Repeat("─", 52),
		"Título:",
		m.titleInput.View(),
		"",
		"Descripción:",
		m.descInput.View(),
		"",
		"Tags (separados por coma):",
		m.tagsInput.View(),
		"",
		fmt.Sprintf("Delegable a IA: %s%s (Espacio para alternar)", checkMark, checkFocus),
	}

	if m.isEditForm {
		statusFocus := ""
		if m.formFocusIdx == 4 {
			statusFocus = " <"
		}
		lines = append(lines, fmt.Sprintf("Estado:         [ %s ]%s (<- / -> o Espacio)", strings.ToUpper(string(m.formStatus)), statusFocus))
	}

	lines = append(lines,
		"",
		saveBtn+"   "+cancelBtn,
		"",
		footerStyle.Render("Usa Tab / Shift+Tab para navegar campos, Ctrl+S o Enter para guardar, Esc para cancelar."),
	)
	if m.errMsg != "" {
		lines = append(lines, errStyle.Render("Error: "+m.errMsg))
	}

	modalW := minInt(m.width-6, 68)
	if modalW < 40 {
		modalW = 40
	}
	return "\n" + modalBoxStyle.Width(modalW).Render(strings.Join(lines, "\n"))
}

func (m Model) renderConfirmDeleteModal() string {
	sel := m.SelectedTask()
	if sel == nil {
		return m.renderBoard()
	}
	content := strings.Join([]string{
		lipgloss.NewStyle().Bold(true).Render("Confirmar Eliminación"),
		"",
		fmt.Sprintf("¿Estás seguro de eliminar la tarea #%d (%q)?", sel.ID, sel.Title),
		"",
		"[y / Enter] Sí, eliminar    [n / Esc] Cancelar",
	}, "\n")
	return "\n" + modalBoxStyle.Width(56).Render(content)
}

func truncateText(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
