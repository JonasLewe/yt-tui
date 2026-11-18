package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"yt-tui/internal/ytcli"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCreateIssueWithSameNameTags(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var created struct {
		Tags []struct{ ID string }
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/projects":
			fmt.Fprint(w, `[{"id":"0-1","shortName":"DEMO"}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/tags":
			if r.URL.Query().Get("$skip") != "0" {
				fmt.Fprint(w, "[]")
				return
			}
			fmt.Fprint(w, `[{"id":"t1","name":"Review","owner":{"login":"alex"}},{"id":"t2","name":"Review","owner":{"login":"sam"}}]`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/issues":
			if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
				t.Error(err)
			}
			fmt.Fprint(w, `{"id":"1-1","idReadable":"DEMO-1"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &ytcli.Client{}
	client.SetCredentials(server.URL, "test-token")
	m := newFormModel(client, nil)
	m.setupForm("DEMO")
	m.width, m.height = 80, 24
	m.summaryInput.SetValue("Review")
	m, _ = m.Update(m.loadCreateTags()())
	for i := 0; i < int(numFields) && m.focusIndex != fieldTags; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	}
	if m.focusIndex != fieldTags || !strings.Contains(m.View(), "Review (alex)") {
		t.Fatalf("tag picker missing after tabbing through the form:\n%s", m.View())
	}

	for _, msg := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("/")}, {Type: tea.KeyRunes, Runes: []rune("SAM")},
		{Type: tea.KeyEnter}, {Type: tea.KeySpace},
	} {
		m, _ = m.Update(msg)
	}
	if !slices.Equal(m.createTags.tagIDs, []string{"t2"}) || m.createTags.tagSearching {
		t.Fatalf("search by owner selected %v", m.createTags.tagIDs)
	}
	for _, msg := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("/")}, {Type: tea.KeyEsc},
		{Type: tea.KeySpace}, {Type: tea.KeySpace}, {Type: tea.KeySpace},
	} {
		m, _ = m.Update(msg)
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if got := cmd().(formSubmittedMsg); got.err != nil || got.issueID != "DEMO-1" {
		t.Fatalf("save result: %+v", got)
	}
	if len(created.Tags) != 2 || created.Tags[0].ID != "t2" || created.Tags[1].ID != "t1" {
		t.Fatalf("posted tag IDs = %+v", created.Tags)
	}
}

func TestCreateTagSearchReceivesGlobalShortcutKeys(t *testing.T) {
	app := AppModel{state: stateForm, form: newFormModel(nil, nil)}
	app.form.setupForm("DEMO")
	app.form.blurCurrent()
	app.form.focusIndex = fieldTags
	for _, key := range "/S?q" {
		next, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		app = next.(AppModel)
	}
	if app.searchShow || app.helpShow || app.form.createTags.tagSearch.Value() != "S?q" {
		t.Fatal("global shortcuts intercepted tag search")
	}
}

func TestCreateTagPickerIgnoresStaleResponsesAndSkipsEditClone(t *testing.T) {
	for _, mode := range []string{"edit:", "clone:"} {
		m := newFormModel(nil, nil)
		m.setupForm("DEMO")
		stale := createTagsLoadedMsg{form: m.createTags.form, tags: []ytcli.Tag{{ID: "1", Name: "Old"}}}
		m.setupForm(mode + "DEMO-1")
		m, _ = m.Update(stale)
		m.loading = false
		if m.createTags.tagsLoaded || strings.Contains(m.View(), "Tags:") {
			t.Fatalf("tag picker restored in %s form", mode)
		}
		m.focusIndex = fieldPriority
		m.nextField()
		if m.focusIndex == fieldTags {
			t.Fatal("reached hidden tag picker moving forward")
		}
		m.focusIndex = fieldAssignee
		m.prevField()
		if m.focusIndex == fieldTags {
			t.Fatal("reached hidden tag picker moving backward")
		}
	}
}
