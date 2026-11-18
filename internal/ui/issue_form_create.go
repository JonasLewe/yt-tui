package ui

import "yt-tui/internal/ytcli"

func (m formModel) createIssue(project, summary, priority, issueType string) (string, error) {
	var options []ytcli.IssueCreateOption
	description := m.descTextArea.Value()
	login := m.assigneeInput.Value()
	return m.client.CreateIssue(project, summary, description, priority, issueType, login, options...)
}
