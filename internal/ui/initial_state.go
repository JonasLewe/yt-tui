package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"yt-tui/internal/ytcli"
)

type initialStateSelector struct {
	project string
	request uint64
	choices ytcli.ProjectStates
	index   int // Zero delegates the initial state to the project's workflow.
	loading bool
	err     error
}

type initialStatesLoadedMsg struct {
	request uint64
	choices ytcli.ProjectStates
	err     error
}

func (m *formModel) loadInitialStates(projectID string) tea.Cmd {
	m.initialState = initialStateSelector{project: projectID, request: m.initialState.request + 1, loading: true}
	client, request := m.client, m.initialState.request
	return func() tea.Msg {
		choices, err := client.GetProjectStates(projectID)
		return initialStatesLoadedMsg{request: request, choices: choices, err: err}
	}
}

func (s initialStateSelector) label() string {
	switch {
	case s.err != nil:
		return "Project default · choices unavailable (Enter to retry)"
	case s.loading:
		return "Project default · loading choices…"
	case s.index > 0:
		return "◀  " + s.choices.Values[s.index-1].Name + "  ▶"
	case len(s.choices.Values) > 0:
		return "◀  Project default  ▶"
	default:
		return "Project default"
	}
}

// Update keeps the selected state scoped to the active project and issue type.
func (m formModel) Update(msg tea.Msg) (formModel, tea.Cmd) {
	if loaded, ok := msg.(initialStatesLoadedMsg); ok {
		if loaded.request == m.initialState.request {
			m.initialState.choices, m.initialState.err = loaded.choices, loaded.err
			m.initialState.loading = false
		}
		return m, nil
	}
	previousType := m.typeIndex
	m, cmd := m.updateForm(msg)
	if m.isClone || m.isEdit || len(m.projects) == 0 {
		return m, cmd
	}
	project := m.projects[m.projectIndex].ID
	if project != "" && project != m.initialState.project {
		if m.initialState.project != "" {
			m.typeIndex, m.priorityIndex = 0, 0
		}
		statesCmd := m.loadInitialStates(project)
		return m, tea.Batch(cmd, statesCmd)
	}
	if previousType != m.typeIndex {
		m.initialState.index = 0
	}
	return m, cmd
}

func (m *formModel) updateStateKey(key string) (bool, tea.Cmd) {
	s := &m.initialState
	count := len(s.choices.Values) + 1
	switch key {
	case "left", "h":
		s.index = (s.index + count - 1) % count
	case "right", "l":
		s.index = (s.index + 1) % count
	case "enter":
		if s.err != nil {
			return true, m.loadInitialStates(s.project)
		}
		return false, nil
	default:
		return false, nil
	}
	return true, nil
}
