package ui

import (
	"strings"
	"testing"
	"yt-tui/internal/ytcli"

	"github.com/charmbracelet/bubbletea"
)

func TestFormLoadsAssigneeLogin(t *testing.T) {
	for _, mode := range []string{"edit", "clone"} {
		for _, login := range []string{"JSmith42", ""} {
			t.Run(mode+"/"+login, func(t *testing.T) {
				m := newFormModel(nil, nil)
				m.isEdit = mode == "edit"
				m.isClone = mode == "clone"
				issue := &ytcli.Issue{ID: "1-1", Summary: "Example"}
				var assignee interface{}
				if login != "" {
					assignee = map[string]interface{}{"login": login, "fullName": "Jane Smith"}
				}
				issue.CustomFields = []ytcli.CustomField{{Name: "Assignee", Value: assignee}}
				m, _ = m.Update(formDataLoadedMsg{issue: issue})
				if got := m.assigneeInput.Value(); got != login {
					t.Fatalf("assignee = %q, want API login %q", got, login)
				}
			})
		}
	}
}

func TestFormAssigneeSuggestionUsesLogin(t *testing.T) {
	for _, key := range []tea.KeyType{tea.KeyTab, tea.KeyEnter} {
		msg := tea.KeyMsg{Type: key}
		t.Run(msg.String(), func(t *testing.T) {
			m := newFormModel(nil, nil)
			m.focusIndex = fieldAssignee
			m.focusCurrent()
			m.filteredUsers = []ytcli.User{{Login: "JSmith42", FullName: "Jane Smith"}}
			m, _ = m.Update(msg)
			if got := m.assigneeInput.Value(); got != "JSmith42" {
				t.Fatalf("selected assignee = %q, want API login JSmith42", got)
			}
			if len(m.filteredUsers) != 0 {
				t.Fatal("suggestions should close after selecting an assignee")
			}
		})
	}
}

func TestFormDescriptionArrowKeysKeepContent(t *testing.T) {
	m := newFormModel(nil, nil)
	m.width = 120
	m.loading = false
	m.focusIndex = fieldDescription
	m.focusCurrent()

	// A long description whose every line is identical so any rendered content
	// line matches the substring (blank/end-of-buffer padding lines do not).
	line := "A very long line of description text that wraps around the textarea."
	desc := strings.TrimSuffix(strings.Repeat(line+"\n", 30), "\n")
	m.descTextArea.SetValue(desc)

	// Render pass: applies the target width to the rendering copy.
	m.View()

	// Press down arrow a few times.
	for i := 0; i < 5; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}

	// The stored value must never change.
	if got := m.descTextArea.Value(); got != desc {
		t.Fatalf("description content was lost after arrow navigation\n got: %q\nwant: %q", got, desc)
	}

	// The rendered view must still show actual description content rather than
	// blank end-of-buffer padding.
	view := m.View()
	if !strings.Contains(view, line) {
		t.Log(view)
		t.Fatal("description not rendered after arrow navigation (viewport lost content)")
	}
}
