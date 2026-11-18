package ui

import (
	"slices"
	"strings"
	"testing"
	"yt-tui/internal/ytcli"

	"github.com/charmbracelet/bubbletea"
)

func TestCreateFieldsFollowProjectAndRetainUnassignedOnTab(t *testing.T) {
	m := newFormModel(nil, nil)
	m.setupForm("DEMO")
	m, _ = m.Update(projectsLoadedMsg{form: m.formNumber, projects: []ytcli.Project{
		{ID: "0-1", ShortName: "DEMO"}, {ID: "0-2", ShortName: "NEXT"},
	}})
	meta := createFieldsLoadedMsg{form: m.formNumber, version: m.createFields.version, project: "DEMO", fields: ytcli.IssueCreateFields{States: []ytcli.CreateState{{ID: "s1", Name: "Ready"}}}}
	m, _ = m.Update(meta)
	m, _ = m.Update(usersLoadedMsg{form: m.formNumber, project: "DEMO", users: []ytcli.User{{Login: "somebody"}}})
	m.focusIndex = fieldAssignee
	m.focusCurrent()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.assigneeInput.Value() != "" || m.focusIndex != fieldState || m.createFields.stateIndex != 0 {
		t.Fatal("tabbing through Assignee changed the new issue defaults")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.createFields.stateIndex != 1 {
		t.Fatal("could not select a project state")
	}
	m.focusIndex = fieldProject
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m, _ = m.Update(meta)
	if m.createFields.project != "NEXT" || m.createFields.stateIndex != 0 || m.createFields.metaReady || len(m.createFields.meta.States) != 0 {
		t.Fatal("previous project's state restored by late response")
	}
}

func TestCreateTagSearchSelectsIdentities(t *testing.T) {
	m := newFormModel(nil, nil)
	m.setupForm("DEMO")
	m.focusIndex = fieldTags
	m, _ = m.Update(createTagsLoadedMsg{form: m.formNumber, tags: []ytcli.Tag{
		{ID: "t1", Name: "Review", Owner: &ytcli.User{Login: "alex"}},
		{ID: "t2", Name: "Review", Owner: &ytcli.User{Login: "sam"}},
	}})
	for _, msg := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("/")},
		{Type: tea.KeyRunes, Runes: []rune("sam")},
		{Type: tea.KeyEnter},
		{Type: tea.KeySpace},
	} {
		m, _ = m.Update(msg)
	}
	if !slices.Equal(m.createFields.tagIDs, []string{"t2"}) || m.createFields.tagSearching {
		t.Fatalf("search selected wrong tag: %+v", m.createFields.tagIDs)
	}
	for _, msg := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("/")},
		{Type: tea.KeyEsc},
		{Type: tea.KeySpace},
	} {
		m, _ = m.Update(msg)
	}
	if !slices.Equal(m.createFields.tagIDs, []string{"t2", "t1"}) {
		t.Fatalf("same-name tags collapsed to %v", m.createFields.tagIDs)
	}
}

func TestChangingIssueTypeRestoresWorkflowState(t *testing.T) {
	m := newFormModel(nil, nil)
	m.focusIndex = fieldType
	m.createFields.stateIndex = 1
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.typeIndex == 0 || m.createFields.stateIndex != 0 {
		t.Fatal("changing issue type must restore the project's initial state")
	}
}

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
			m.assigneeInput.SetValue("Jane")
			m.allUsers = []ytcli.User{{Login: "JSmith42", FullName: "Jane Smith"}}
			m.focusCurrent()
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
