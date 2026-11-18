package ui

import "yt-tui/internal/ytcli"

func (m formModel) createIssue(project, summary, priority, issueType string) (string, error) {
	var options []ytcli.IssueCreateOption
	return m.client.CreateIssue(project, summary, m.descTextArea.Value(), priority, issueType, m.assigneeInput.Value(), options...)
}
