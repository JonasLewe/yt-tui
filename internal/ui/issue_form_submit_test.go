package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"yt-tui/internal/ytcli"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFormRetryAfterIssueWasCreated(t *testing.T) {
	var creates, links, uploads int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/admin/projects":
			fmt.Fprint(w, `[{"id":"0-1","shortName":"APP"}]`)
		case "/api/issues":
			creates++
			fmt.Fprint(w, `{"idReadable":"APP-2"}`)
		case "/api/issueLinkTypes":
			fmt.Fprint(w, `[{"id":"80-1","targetToSource":"is clone of","directed":true}]`)
		case "/api/issues/APP-2/links/80-1t/issues":
			links++
			w.WriteHeader(http.StatusNoContent)
		case "/api/issues/APP-2/attachments":
			uploads++
			if uploads == 2 {
				http.Error(w, "try again", http.StatusServiceUnavailable)
			}
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := ytcli.NewClient()
	c.SetCredentials(server.URL, "test")
	m := newFormModel(c, nil)
	m.isClone, m.cloneIssueID, m.initialProjectCode = true, "2-1", "APP"
	m.summaryInput.SetValue("Follow up")
	m.pastedImages = []PastedImage{{Name: "first.png"}, {Name: "second.png"}}

	save := tea.KeyMsg{Type: tea.KeyCtrlS}
	m, start := m.Update(save)
	if start == nil {
		t.Fatal("missing create command")
	}
	m, _ = m.Update(start())
	if m.pendingSave == nil || !strings.Contains(m.View(), "Saved APP-2") {
		t.Fatalf("partial save did not retain the created issue: %v", m.err)
	}
	m, retry := m.Update(save)
	if retry == nil {
		t.Fatal("missing retry command")
	}
	if _, duplicate := m.Update(save); duplicate != nil {
		t.Fatal("second key started another save during retry")
	}
	m, success := m.Update(retry())
	if m.pendingSave != nil || success == nil || success().(cloneSubmittedMsg).issueID != "APP-2" {
		t.Fatalf("retry didn't finish on original issue: %v", m.err)
	}
	if creates != 1 || links != 1 || uploads != 3 {
		t.Fatalf("requests after retry: creates=%d links=%d uploads=%d", creates, links, uploads)
	}
}

func TestRetryFailedLinkOnNewIssue(t *testing.T) {
	var creates, firstLink, secondLink int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/admin/projects":
			fmt.Fprint(w, `[{"id":"0-1","shortName":"APP"}]`)
		case "/api/admin/projects/0-1/customFields":
			fmt.Fprint(w, `[{"$type":"UserProjectCustomField","canBeEmpty":true,"field":{"name":"Assignee","fieldType":{"isMultiValue":false}}}]`)
		case "/api/issues":
			creates++
			fmt.Fprint(w, `{"idReadable":"APP-3"}`)
		case "/api/issues/APP-3/links/80-1t/issues":
			firstLink++
			w.WriteHeader(http.StatusNoContent)
		case "/api/issues/APP-3/links/80-2s/issues":
			secondLink++
			if secondLink == 1 {
				http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := ytcli.NewClient()
	c.SetCredentials(server.URL, "test")
	m := newFormModel(c, nil)
	m.width, m.height = 80, 24
	m.initialProjectCode = "APP"
	m.summaryInput.SetValue("Test child")
	m.links.selected = []formIssueLink{
		{RelationID: "80-1t", Label: "subtask of", TargetID: "2-1", TargetReadable: "APP-1"},
		{RelationID: "80-2s", Label: "depends on", TargetID: "2-2", TargetReadable: "APP-2"},
	}

	m, save := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if save == nil {
		t.Fatal("missing command to create the linked issue")
	}
	m, _ = m.Update(save())
	if m.pendingSave == nil || !strings.Contains(m.View(), "Saved APP-3") || !strings.Contains(m.View(), "APP-2") {
		t.Fatalf("failed link and created issue should both be identifiable: %s", m.View())
	}
	m, retry := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if retry == nil {
		t.Fatal("missing command to retry the failed link")
	}
	m, done := m.Update(retry())
	if done == nil || m.pendingSave != nil {
		t.Fatalf("retry did not finish: %v", m.err)
	}
	if back, ok := done().(popStateMsg); !ok || back.projectCodeToInvalidate != "APP" {
		t.Fatalf("missing refresh of project APP: %+v", back)
	}
	if creates != 1 || firstLink != 1 || secondLink != 2 {
		t.Fatalf("requests after retry: creates=%d first link=%d second link=%d", creates, firstLink, secondLink)
	}
}
