package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"taskflow/internal/domain"
)

func (m *Model) openCreateForm() {
	m.mode = ModeForm
	m.isEditForm = false
	m.editingTaskID = 0
	m.formFocusIdx = 0

	m.titleInput.SetValue("")
	m.descInput.SetValue("")
	m.tagsInput.SetValue("")
	m.delegableToAI = true
	m.formStatus = domain.StatusPending

	m.syncFormFocus()
}

func (m *Model) openEditForm(task *domain.Task) {
	if task == nil {
		return
	}
	m.mode = ModeForm
	m.isEditForm = true
	m.editingTaskID = task.ID
	m.formFocusIdx = 0

	m.titleInput.SetValue(task.Title)
	m.descInput.SetValue(task.Description)
	m.tagsInput.SetValue(strings.Join(task.Tags, ", "))
	m.delegableToAI = task.DelegableToAI
	m.formStatus = task.Status
	if !m.formStatus.IsValid() {
		m.formStatus = domain.StatusPending
	}

	m.syncFormFocus()
}

func (m *Model) cycleFormStatus(delta int) {
	idx := 0
	for i, s := range ColumnOrder {
		if s == m.formStatus {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(ColumnOrder)) % len(ColumnOrder)
	m.formStatus = ColumnOrder[idx]
}

func (m *Model) advanceFormFocus(delta int) {
	const totalFields = 7 // 0=Title, 1=Desc, 2=Tags, 3=Delegable, 4=Status(edit only), 5=Save, 6=Cancel
	m.formFocusIdx = (m.formFocusIdx + delta + totalFields) % totalFields
	if !m.isEditForm && m.formFocusIdx == 4 {
		m.formFocusIdx = (m.formFocusIdx + delta + totalFields) % totalFields
	}
	m.syncFormFocus()
}

func (m *Model) syncFormFocus() {
	m.titleInput.Blur()
	m.descInput.Blur()
	m.tagsInput.Blur()

	switch m.formFocusIdx {
	case 0:
		m.titleInput.Focus()
	case 1:
		m.descInput.Focus()
	case 2:
		m.tagsInput.Focus()
	}
}

func (m *Model) parseFormTags() []string {
	raw := m.tagsInput.Value()
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	parts := strings.Split(raw, ",")
	tags := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			tags = append(tags, trimmed)
		}
	}
	return tags
}

func (m *Model) updateFormInputs(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	switch m.formFocusIdx {
	case 0:
		var cmd tea.Cmd
		m.titleInput, cmd = m.titleInput.Update(msg)
		cmds = append(cmds, cmd)
	case 1:
		var cmd tea.Cmd
		m.descInput, cmd = m.descInput.Update(msg)
		cmds = append(cmds, cmd)
	case 2:
		var cmd tea.Cmd
		m.tagsInput, cmd = m.tagsInput.Update(msg)
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
}
