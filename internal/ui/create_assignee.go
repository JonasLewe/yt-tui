package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"yt-tui/internal/ytcli"
)

type createAssignee struct {
	request uint64
	project string
	field   *ytcli.ProjectAssignee
}

type projectAssigneeLoadedMsg struct {
	request uint64
	field   ytcli.ProjectAssignee
}

func (m *formModel) refreshAssignee() tea.Cmd {
	project := m.getSelectedProjectCode()
	if project == "" || project == m.createAssignee.project {
		return nil
	}
	if !m.isClone && !m.isEdit && m.createAssignee.project != "" {
		m.typeIndex, m.priorityIndex = 0, 0
	}
	m.createAssignee = createAssignee{request: m.createAssignee.request + 1, project: project}
	m.allUsers, m.filteredUsers = nil, nil
	users := m.loadUsersCmd(project)
	if m.isEdit || m.isClone || len(m.projects) == 0 {
		return users
	}
	m.assigneeInput.Placeholder = "Unassigned"
	projectID := m.projects[m.projectIndex].ID
	client, request := m.client, m.createAssignee.request
	return tea.Batch(users, func() tea.Msg {
		field, _ := client.GetProjectAssignee(projectID)
		return projectAssigneeLoadedMsg{request: request, field: field}
	})
}

func (m formModel) noAssigneeField() bool {
	return !m.isClone && !m.isEdit && m.createAssignee.field != nil && m.createAssignee.field.Name == ""
}

func (m formModel) newIssueAssignee(project string) (ytcli.IssueCreateOption, error) {
	field := m.createAssignee.field
	if field == nil || m.createAssignee.project != project {
		// Quick saves can arrive before the project list or field metadata.
		var projectID string
		if len(m.projects) > 0 && m.projects[m.projectIndex].ShortName == project {
			projectID = m.projects[m.projectIndex].ID
		} else {
			p, err := m.client.ResolveIssueTypeProject(project)
			if err != nil {
				return nil, err
			}
			projectID = p.ID
		}
		f, _ := m.client.GetProjectAssignee(projectID)
		field = &f
	}
	return m.client.NewIssueAssignee(*field, m.assigneeInput.Value())
}
