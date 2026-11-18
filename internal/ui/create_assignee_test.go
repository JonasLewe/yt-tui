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

func TestNewIssueAssigneeNavigation(t *testing.T) {
	m := newFormModel(nil, nil)
	m.setupForm("DEMO")
	m, _ = m.Update(projectsLoadedMsg{projects: []ytcli.Project{{ShortName: "DEMO", ID: "0-1"}, {ShortName: "NEXT", ID: "0-2"}}})
	request := m.createAssignee.request
	m, _ = m.Update(usersLoadedMsg{request: request, users: []ytcli.User{{Login: "JSmith", FullName: "Jane Smith"}}})
	m.focusIndex = fieldAssignee
	m.focusCurrent()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.assigneeInput.Value() != "" || m.focusIndex == fieldAssignee {
		t.Fatal("tabbing through the empty field accepted a user suggestion")
	}

	m.focusIndex = fieldAssignee
	m.focusCurrent()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Jane")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.assigneeInput.Value() != "JSmith" {
		t.Fatalf("typed search selected %q", m.assigneeInput.Value())
	}

	m.focusIndex = fieldProject
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m, _ = m.Update(projectAssigneeLoadedMsg{request: request, field: ytcli.ProjectAssignee{Name: "Assignee", Required: true}})
	if m.createAssignee.field != nil {
		t.Fatal("accepted stale project metadata")
	}
	m, _ = m.Update(projectAssigneeLoadedMsg{request: m.createAssignee.request})
	m.focusIndex = fieldPriority
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focusIndex == fieldAssignee || m.assigneeInput.Value() != "" {
		t.Fatal("project without an Assignee field retained the previous assignee")
	}
}

func TestCreateAssigneeBeforeMetadataArrives(t *testing.T) {
	for _, tc := range []struct {
		name, metadata, login, want string
		blocked                     bool
	}{
		{"unassigned", `{"$type":"UserProjectCustomField","canBeEmpty":true,"field":{"name":"Assignee"}}`, "", `{"$type":"SingleUserIssueCustomField","name":"Assignee","value":null}`, false},
		{"multi empty", `{"$type":"UserProjectCustomField","canBeEmpty":true,"field":{"name":"Assignee","fieldType":{"isMultiValue":true}}}`, "unassigned", `{"$type":"MultiUserIssueCustomField","name":"Assignee","value":[]}`, false},
		{"active login", `{"$type":"UserProjectCustomField","canBeEmpty":false,"field":{"name":"Assignee","fieldType":{"isMultiValue":true}}}`, "Jörg.Smith", `{"$type":"MultiUserIssueCustomField","name":"Assignee","value":[{"login":"Jörg.Smith"}]}`, false},
		{"required", `{"$type":"UserProjectCustomField","canBeEmpty":false,"field":{"name":"Assignee"}}`, "", "", true},
		{"missing", `{"$type":"UserProjectCustomField","canBeEmpty":true,"field":{"name":"Reviewer"}}`, "", "", false},
		{"limited access", "", "", `{"$type":"SingleUserIssueCustomField","name":"Assignee","value":null}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var posts int
			var field json.RawMessage
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/admin/projects":
					fmt.Fprint(w, `[{"id":"0-1","shortName":"DEMO"}]`)
				case "/api/admin/projects/0-1/customFields":
					if tc.metadata == "" {
						http.Error(w, "restricted", http.StatusForbidden)
						return
					}
					fmt.Fprintf(w, "[%s]", tc.metadata)
				case "/api/issues":
					posts++
					var payload struct{ CustomFields []json.RawMessage }
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
					}
					for _, f := range payload.CustomFields {
						if strings.Contains(string(f), `"name":"Assignee"`) {
							field = f
						}
					}
					fmt.Fprint(w, `{"idReadable":"DEMO-1"}`)
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			// Exercise a quick save in a project opened directly from the issues list.
			client := &ytcli.Client{}
			client.SetCredentials(srv.URL, "test")
			m := newFormModel(client, nil)
			m.setupForm("DEMO")
			m.summaryInput.SetValue("Feature")
			m.assigneeInput.SetValue(tc.login)
			_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
			msg := cmd().(formSubmittedMsg)
			if tc.blocked {
				if posts != 0 || msg.err == nil || !strings.Contains(msg.err.Error(), "required") {
					t.Fatalf("required user: %d posts, %v", posts, msg.err)
				}
			} else if posts != 1 || msg.err != nil || msg.issueID != "DEMO-1" || string(field) != tc.want {
				t.Fatalf("create: %d posts, %s, %v, field %s", posts, msg.issueID, msg.err, field)
			}
		})
	}
}
