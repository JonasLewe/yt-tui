package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"yt-tui/internal/ytcli"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func articleFormKey(key string) tea.KeyMsg {
	switch key {
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	case "ctrl+g":
		return tea.KeyMsg{Type: tea.KeyCtrlG}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
}

func articleFormCommandMessages(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var messages []tea.Msg
		for _, child := range batch {
			messages = append(messages, articleFormCommandMessages(child)...)
		}
		return messages
	}
	switch msg.(type) {
	case articleFormDataMsg, articleFormSubmittedMsg, articleSavedMsg, popStateMsg:
		return []tea.Msg{msg}
	}
	return nil
}

func articleFormDeliver(m articleFormModel, cmd tea.Cmd) (articleFormModel, tea.Cmd) {
	var next tea.Cmd
	for _, msg := range articleFormCommandMessages(cmd) {
		m, next = m.Update(msg)
	}
	return m, next
}

func TestArticleFormCreateUsesContextAndOptionalParent(t *testing.T) {
	for _, child := range []bool{false, true} {
		t.Run(fmt.Sprintf("child=%t", child), func(t *testing.T) {
			var payload map[string]interface{}
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/articles/226-1":
					fmt.Fprint(w, `{"id":"226-1","idReadable":"DEMO-A-1","summary":"Handbook","project":{"shortName":"DEMO"}}`)
				case r.Method == http.MethodGet && r.URL.Path == "/api/admin/projects":
					fmt.Fprint(w, `[{"id":"0-1","shortName":"DEMO"}]`)
				case r.Method == http.MethodPost && r.URL.Path == "/api/articles":
					writes++
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
					}
					fmt.Fprint(w, `{"id":"226-2","idReadable":"DEMO-A-2"}`)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			client := &ytcli.Client{}
			client.SetCredentials(server.URL, "test-token")
			m := newArticleFormModel(client, nil)
			m.width, m.height = 100, 35
			cmd := m.setupForm("new:DEMO:226-1")
			m, _ = articleFormDeliver(m, cmd)
			if m.parentSelected || m.parentCandidate == nil {
				t.Fatal("new article should offer the selected parent without automatically nesting")
			}
			m.summaryInput.SetValue("New article")
			m.contentInput.SetValue("# Notes\n\nExact content")
			if child {
				m.focusIndex = articleFieldParent
				m.focusCurrent()
				m, _ = m.Update(articleFormKey("l"))
			}
			m, cmd = m.Update(articleFormKey("ctrl+s"))
			if !m.saving {
				t.Fatal("Ctrl+S did not start submission")
			}
			_, duplicate := m.Update(articleFormKey("ctrl+s"))
			if duplicate != nil {
				t.Fatal("second Ctrl+S started a concurrent submission")
			}
			m, navigation := articleFormDeliver(m, cmd)
			if m.err != nil || writes != 1 || payload["summary"] != "New article" || payload["content"] != "# Notes\n\nExact content" {
				t.Fatalf("unexpected submission: writes=%d error=%v payload=%v", writes, m.err, payload)
			}
			_, duplicate = m.Update(articleFormKey("ctrl+s"))
			if duplicate != nil {
				t.Fatal("Ctrl+S after success but before parent navigation started a second submission")
			}
			parent, hasParent := payload["parentArticle"].(map[string]interface{})
			if hasParent != child || (child && parent["id"] != "226-1") {
				t.Fatalf("unexpected parent payload: %v", payload["parentArticle"])
			}
			if navigation == nil {
				t.Fatal("successful submission did not request navigation")
			}
			saved, ok := navigation().(articleSavedMsg)
			if !ok || saved.projectCode != "DEMO" || saved.articleID == "" || saved.requestID != m.requestID {
				t.Fatalf("unexpected save message: %+v", saved)
			}
		})
	}
}

func TestArticleFormEditFailurePreservesDraftAndRetry(t *testing.T) {
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/api/articles/226-1" {
			fmt.Fprint(w, `{"id":"226-1","summary":"Original","content":"Original body","project":{"shortName":"DEMO"},"parentArticle":{"id":"226-0"}}`)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/articles/226-1" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		writes++
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload["summary"] != "Changed" || payload["content"] != "" || len(payload) != 2 {
			t.Errorf("edit should only update title and exact content: %v", payload)
		}
		if writes == 1 {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":"Forbidden","error_description":"Update Article permission required"}`)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client := &ytcli.Client{}
	client.SetCredentials(server.URL, "test-token")
	m := newArticleFormModel(client, nil)
	load := m.setupForm("edit:226-1")
	m, _ = articleFormDeliver(m, load)
	if !m.ready || m.summaryInput.Value() != "Original" || m.contentInput.Value() != "Original body" {
		t.Fatal("edit did not load article content")
	}
	m.summaryInput.SetValue("Changed")
	m.contentInput.SetValue("")
	m, save := m.Update(articleFormKey("ctrl+s"))
	m, navigation := articleFormDeliver(m, save)
	if !m.errPopupShow || m.err == nil || navigation != nil || m.saving || m.summaryInput.Value() != "Changed" || m.contentInput.Value() != "" {
		t.Fatal("failed save lost the draft or left the form")
	}
	m, _ = m.Update(articleFormKey("esc")) // Existing form workflow: dismiss popup first.
	m, save = m.Update(articleFormKey("ctrl+s"))
	m, navigation = articleFormDeliver(m, save)
	if m.err != nil || navigation == nil || writes != 2 {
		t.Fatal("retry did not save the preserved draft")
	}
}

func TestArticleFormNavigationAndEditorCompletion(t *testing.T) {
	m := newArticleFormModel(nil, nil)
	m.width, m.height = 100, 35
	m.setupForm("DEMO")
	m.summaryInput.SetValue("Draft")
	content := strings.Repeat("line\n", 150)
	m.contentInput.SetValue(content)
	if m.contentInput.Value() != content {
		t.Fatal("long article content was truncated")
	}
	m, _ = m.Update(articleFormKey("tab"))
	if !m.contentInput.Focused() || m.summaryInput.Focused() {
		t.Fatal("Tab did not focus article content")
	}
	m, _ = m.Update(articleFormKey("down"))
	if m.focusIndex != articleFieldContent || m.contentInput.Value() != content {
		t.Fatal("Down left the content field or changed content")
	}
	m, cmd := m.Update(articleFormKey("ctrl+g"))
	if cmd == nil || !m.editorPending {
		t.Fatal("Ctrl+G did not request the shared editor")
	}
	path := filepath.Join(t.TempDir(), "article.md")
	if err := os.WriteFile(path, []byte("Editor result\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m, _ = m.Update(editorFinishedMsg{tempPath: path})
	if m.contentInput.Value() != "Editor result\n" || m.saving || m.editorPending {
		t.Fatal("editor completion should only update the unsaved draft")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("editor temporary file was not removed")
	}
	m, _ = m.Update(articleFormKey("shift+tab"))
	if !m.summaryInput.Focused() {
		t.Fatal("Shift+Tab did not restore summary focus")
	}
	_, cmd = m.Update(articleFormKey("esc"))
	if cmd == nil {
		t.Fatal("Escape did not cancel the form")
	}
	if _, ok := cmd().(popStateMsg); !ok {
		t.Fatal("Escape performed an action other than navigating back")
	}
}

func TestArticleFormRejectsInvalidAndStaleOperations(t *testing.T) {
	m := newArticleFormModel(nil, nil)
	m.setupForm("edit:226-1")
	old := m.requestID
	m.setupForm("edit:226-2")
	m, _ = m.Update(articleFormDataMsg{requestID: old, article: &ytcli.Article{ID: "226-1", Summary: "stale"}})
	if m.summaryInput.Value() != "" || !m.loading {
		t.Fatal("stale load replaced the current form")
	}
	m, _ = m.Update(articleFormDataMsg{requestID: m.requestID, err: errors.New("read denied")})
	m, _ = m.Update(articleFormKey("esc"))
	m.summaryInput.SetValue("Cannot overwrite an unread article")
	m, cmd := m.Update(articleFormKey("ctrl+s"))
	if cmd != nil || !m.errPopupShow || m.saving {
		t.Fatal("edit was allowed after its initial read failed")
	}
	m.setupForm("DEMO")
	m.summaryInput.SetValue(" \t ")
	m, cmd = m.Update(articleFormKey("ctrl+s"))
	if cmd != nil || !m.errPopupShow {
		t.Fatal("empty summary was accepted")
	}
	m.setupForm("DEMO")
	m.saving, m.loading = true, true
	m, cmd = m.Update(articleFormSubmittedMsg{requestID: m.requestID - 1, articleID: "old"})
	if cmd != nil || !m.saving {
		t.Fatal("stale submission navigated away from the new form")
	}
}

func TestArticleFormEditorFailureKeepsDraftAndCleansFile(t *testing.T) {
	m := newArticleFormModel(nil, nil)
	m.setupForm("DEMO")
	m.contentInput.SetValue("Keep this draft")
	m.editorPending = true
	path := filepath.Join(t.TempDir(), "failed-editor.md")
	if err := os.WriteFile(path, []byte("Partial edit"), 0600); err != nil {
		t.Fatal(err)
	}
	m, _ = m.Update(editorFinishedMsg{tempPath: path, err: errors.New("editor failed")})
	if m.contentInput.Value() != "Keep this draft" || !m.errPopupShow || m.editorPending {
		t.Fatal("editor failure changed the existing draft")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("failed editor's temporary file was not removed")
	}
}

func TestArticleFormKeepsFieldsAndFooterVisible(t *testing.T) {
	for _, size := range [][2]int{{100, 35}, {90, 27}, {60, 25}} {
		m := newArticleFormModel(nil, nil)
		m.width, m.height = size[0], size[1]
		m.setupForm("DEMO")
		m.parentCandidate = &ytcli.Article{ID: "226-1", IDReadable: "DEMO-A-1", Summary: strings.Repeat("Parent ", 20)}
		m.parentSelected = true
		for _, focus := range []articleFormField{articleFieldSummary, articleFieldContent, articleFieldParent} {
			m.focusIndex = focus
			m.focusCurrent()
			view := m.View()
			plain := ansi.Strip(view)
			if !strings.Contains(plain, "Summary:") || !strings.Contains(plain, "Content:") || !strings.Contains(plain, "Parent:") || !strings.Contains(plain, "[Ctrl+s]") {
				t.Errorf("%dx%d focus %d loses fields/footer:\n%s", size[0], size[1], focus, plain)
			}
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Errorf("form exceeds %dx%d viewport", size[0], size[1])
			}
		}
	}
}

func TestArticleFormPreservesRawContentAndHiddenTail(t *testing.T) {
	cases := []string{
		"# Title\r\n\tIndented\r\n",
		strings.Repeat("Long line\n", 10050) + "END\tOF ARTICLE\r\n",
		strings.Repeat("CRLF line\r\n", 5010) + "END OF ARTICLE",
	}
	for i, source := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			m := newArticleFormModel(nil, nil)
			m.setupForm("DEMO")
			m.setContent(source)
			m.summaryInput.SetValue("Changed title")
			if got := m.contentValue(); got != source {
				t.Fatalf("unchanged body lost original formatting or content: lengths %d != %d", len(got), len(source))
			}
			if m.contentTail != "" {
				originalTail := m.contentTail
				editedPrefix := strings.Replace(m.contentInput.Value(), "line", "edited line", 1)
				m.contentInput.SetValue(editedPrefix)
				got := m.contentValue()
				if got != editedPrefix+originalTail {
					t.Fatal("inline editing changed or dropped the undisplayed tail")
				}
			}
			m.editorPending = true
			path := filepath.Join(t.TempDir(), "article.md")
			if err := os.WriteFile(path, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			m, _ = m.Update(editorFinishedMsg{tempPath: path})
			if m.contentValue() != source {
				t.Fatal("editor-returned source was normalized or truncated")
			}
		})
	}
}
