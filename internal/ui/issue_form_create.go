package ui

import "yt-tui/internal/ytcli"

func (m formModel) createIssue(project, summary, priority, issueType string) (string, error) {
	var options []ytcli.IssueCreateOption
	description := m.descTextArea.Value()
	if !m.isEdit && !m.isClone && len(m.createTags.tagIDs) > 0 {
		options = append(options, ytcli.WithIssueTags(m.createTags.tagIDs))
	}
	login := m.assigneeInput.Value()
	return m.client.CreateIssue(project, summary, description, priority, issueType, login, options...)
}
