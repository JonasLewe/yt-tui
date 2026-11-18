package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"yt-tui/internal/ytcli"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestNewIssueFormScrollsToEachField(t *testing.T) {
	m := newFormModel(nil, nil)
	m.width, m.height = 56, 19
	m.initialProjectCode = "APP"
	m.applyInputSizes()
	for _, label := range []string{"Project:", "Summary:", "Description:", "Type:", "Priority:", "Assignee:", "State:", "Tags:", "Links:"} {
		view := m.View()
		if !strings.Contains(view, label) || lipgloss.Height(view) > m.height {
			t.Fatalf("focused %s at %d x %d:\n%s", label, m.width, m.height, view)
		}
		m.nextField()
	}
}

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
