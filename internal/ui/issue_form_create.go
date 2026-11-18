package ui

import "yt-tui/internal/ytcli"

func (m formModel) createIssue(project, summary, priority, issueType string) (string, error) {
	var options []ytcli.IssueCreateOption
	if !m.isClone && !m.isEdit && m.initialState.index > 0 {
		state := m.initialState.choices.Values[m.initialState.index-1]
		options = append(options, ytcli.WithInitialState(m.initialState.choices.FieldName, state.ID))
	}
	description := m.descTextArea.Value()
	if !m.isEdit && !m.isClone && len(m.createTags.tagIDs) > 0 {
		options = append(options, ytcli.WithIssueTags(m.createTags.tagIDs))
	}
	login := m.assigneeInput.Value()
	if !m.isEdit && !m.isClone {
		assignee, err := m.newIssueAssignee(project)
		if err != nil {
			return "", err
		}
		options = append(options, assignee)
		// The option encodes both empty and multi-user fields in the initial request.
		login = ""
	}
	return m.client.CreateIssue(project, summary, description, priority, issueType, login, options...)
}
