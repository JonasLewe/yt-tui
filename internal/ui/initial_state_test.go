package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"yt-tui/internal/ytcli"
)

func TestInitialStateForm(t *testing.T) {
	var saved []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/admin/projects":
			fmt.Fprint(w, `[{"id":"0-1","shortName":"APP"}]`)
		case "/api/admin/projects/0-1/customFields":
			fmt.Fprint(w, `[{"$type":"StateProjectCustomField","field":{"name":"Stage"},"bundle":{"id":"b1"}}]`)
		case "/api/admin/customFieldSettings/bundles/state/b1/values":
			fmt.Fprint(w, `[{"id":"s1","name":"Ready"},{"id":"s2","name":"Retired","archived":true}]`)
		case "/api/issues":
			var payload struct {
				CustomFields []struct {
					Name  string
					Type  string `json:"$type"`
					Value struct{ ID string }
				}
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			state := ""
			for _, f := range payload.CustomFields {
				if f.Name == "Stage" {
					state = f.Value.ID
					if f.Type != "StateIssueCustomField" {
						t.Errorf("unexpected state field type %q", f.Type)
					}
				}
			}
			saved = append(saved, state)
			fmt.Fprint(w, `{"idReadable":"APP-1"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &ytcli.Client{}
	client.SetCredentials(server.URL, "test")
	m := newFormModel(client, nil)
	m.width, m.height = 56, 19
	m.setupForm("APP")
	m, _ = m.Update(projectsLoadedMsg{projects: []ytcli.Project{{ID: "0-1", ShortName: "APP"}, {ID: "0-2", ShortName: "NEXT"}}})
	m.summaryInput.SetValue("Feature")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if submitted := cmd().(formSubmittedMsg); submitted.err != nil {
		t.Fatal(submitted.err)
	}

	load := m.loadInitialStates("0-1")
	loaded := load().(initialStatesLoadedMsg)
	m, _ = m.Update(loaded)
	if loaded.err != nil || len(m.initialState.choices.Values) != 1 {
		t.Fatalf("active project states: %+v, %v", loaded.choices, loaded.err)
	}
	m.focusIndex = fieldState
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if !strings.Contains(m.View(), "Ready") {
		t.Fatal("selected state is not displayed")
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if submitted := cmd().(formSubmittedMsg); submitted.err != nil {
		t.Fatal(submitted.err)
	}
	if len(saved) != 2 || saved[0] != "" || saved[1] != "s1" {
		t.Fatalf("states submitted: %v", saved)
	}

	m.focusIndex = fieldType
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.initialState.index != 0 {
		t.Fatal("changing type retained the previous workflow's state")
	}
	m.focusIndex = fieldProject
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m, _ = m.Update(loaded)
	if m.initialState.project != "0-2" || len(m.initialState.choices.Values) > 0 || m.initialState.index != 0 {
		t.Fatal("stale state data restored after switching project")
	}
}

func TestCloneAndEditSkipInitialState(t *testing.T) {
	for _, mode := range []string{"clone:", "edit:"} {
		m := newFormModel(nil, nil)
		m.setupForm(mode + "APP-1")
		m.loading = false
		m.focusIndex = fieldType
		for _, step := range []func(){m.nextField, m.prevField} {
			step()
			if m.focusIndex == fieldState || strings.Contains(m.View(), "State:") {
				t.Fatalf("%s exposed the initial state selector", mode)
			}
		}
	}
}
