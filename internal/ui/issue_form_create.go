package ui

import "yt-tui/internal/ytcli"

func (m formModel) createIssue(project, summary, priority, issueType string) (string, error) {
	var options []ytcli.IssueCreateOption
	description := m.descTextArea.Value()
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
