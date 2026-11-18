package ui

import "yt-tui/internal/ytcli"

func (m formModel) createIssue(project, summary, priority, issueType string) (string, error) {
	var options []ytcli.IssueCreateOption
	if !m.isClone && !m.isEdit && m.initialState.index > 0 {
		state := m.initialState.choices.Values[m.initialState.index-1]
		options = append(options, ytcli.WithInitialState(m.initialState.choices.FieldName, state.ID))
	}
	description := m.descTextArea.Value()
	login := m.assigneeInput.Value()
	return m.client.CreateIssue(project, summary, description, priority, issueType, login, options...)
}
