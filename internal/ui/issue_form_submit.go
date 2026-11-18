package ui

import (
	"fmt"
	"os"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The issue has already been saved. Keep only the operations that still need
// to run so retrying a link or attachment can't create another issue.
type pendingIssueSave struct {
	issueID string
	steps   []func(string) error
}

func (p pendingIssueSave) finish() tea.Msg {
	for len(p.steps) > 0 {
		if err := p.steps[0](p.issueID); err != nil {
			return formSubmittedMsg{issueID: p.issueID, pending: &p, err: err}
		}
		p.steps = p.steps[1:]
	}
	return formSubmittedMsg{issueID: p.issueID}
}

func (m formModel) submitIssueCmd(project, summary, priority, issueType string) tea.Cmd {
	return func() tea.Msg {
		var id string
		var err error
		if m.isEdit {
			id = m.editKey
			err = m.client.UpdateIssue(id, summary, m.descTextArea.Value(), priority, issueType, m.assigneeInput.Value())
		} else {
			id, err = m.createIssue(project, summary, priority, issueType)
		}
		if err != nil {
			return formSubmittedMsg{err: err}
		}

		pending := pendingIssueSave{issueID: id}
		if m.isClone && m.cloneIssueID != "" {
			pending.steps = append(pending.steps, func(id string) error {
				if err := m.client.LinkClonedIssue(id, m.cloneIssueID); err != nil {
					return fmt.Errorf("link to the original issue: %w", err)
				}
				return nil
			})
		}
		for _, link := range m.links.selected {
			pending.steps = append(pending.steps, func(id string) error {
				if err := m.client.AddIssueLink(id, link.RelationID, link.TargetID); err != nil {
					return fmt.Errorf("link %s to %s (%s): %w", id, link.TargetReadable, link.Label, err)
				}
				return nil
			})
		}
		for _, file := range m.pastedImages {
			pending.steps = append(pending.steps, func(id string) error {
				content := file.Bytes
				if file.Path != "" {
					data, err := os.ReadFile(file.Path)
					if err != nil {
						return fmt.Errorf("read %s: %w", file.Path, err)
					}
					content = data
				}
				if err := m.client.UploadAttachment(id, file.Name, content); err != nil {
					return fmt.Errorf("upload %s: %w", file.Name, err)
				}
				return nil
			})
		}
		return pending.finish()
	}
}

func (m formModel) updatePendingSave(key tea.KeyMsg) (formModel, tea.Cmd) {
	switch key.String() {
	case "ctrl+s", "enter":
		m.loading = true
		return m, m.pendingSave.finish
	case "esc":
		project := m.getSelectedProjectCode()
		return m, func() tea.Msg { return popStateMsg{projectCodeToInvalidate: project} }
	case "y":
		if m.err != nil {
			_ = clipboard.WriteAll(m.err.Error())
		}
	}
	return m, nil
}

func (m formModel) pendingSaveView() string {
	title := fmt.Sprintf("Saved %s", m.pendingSave.issueID)
	if m.loading {
		return title + "\n" + m.spinner.View() + " Retrying…"
	}
	body := lipgloss.NewStyle().Width(max(1, min(76, m.width-4))).Render(m.err.Error())
	return lipgloss.JoinVertical(lipgloss.Left, StyleTitle.Render(title), body, "",
		StyleHelp.Render("[Ctrl+s / Enter] Retry   [Esc] Back   [y] Copy error"))
}
