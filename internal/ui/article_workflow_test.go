package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"yt-tui/internal/config"
	"yt-tui/internal/ytcli"

	tea "github.com/charmbracelet/bubbletea"
)

type articleWorkflowAPI struct {
	mu         sync.Mutex
	articles   []ytcli.Article
	writes     int
	denyWrites bool
}

func newArticleWorkflowApp(t *testing.T) (AppModel, *articleWorkflowAPI) {
	t.Helper()
	fixture := &articleWorkflowAPI{articles: []ytcli.Article{{
		ID: "1-1", IDReadable: "DEMO-A-1", Summary: "Handbook", Content: "Original content",
		Project: &ytcli.Project{ID: "0-1", ShortName: "DEMO"},
	}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixture.mu.Lock()
		defer fixture.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			fixture.writes++
			if fixture.denyWrites {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"access_denied","error_description":"Update Article permission is required"}`))
				return
			}
			var payload ytcli.Article
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode article write: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			switch r.URL.Path {
			case "/api/articles/1-1":
				fixture.articles[0].Summary, fixture.articles[0].Content = payload.Summary, payload.Content
				w.WriteHeader(http.StatusNoContent)
			case "/api/articles":
				payload.ID, payload.IDReadable = "1-2", "DEMO-A-2"
				payload.Project = fixture.articles[0].Project
				fixture.articles = append(fixture.articles, payload)
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(payload)
			default:
				t.Errorf("unexpected article write: %s", r.URL.Path)
				http.NotFound(w, r)
			}
			return
		}
		switch r.URL.Path {
		case "/api/admin/projects":
			_ = json.NewEncoder(w).Encode([]ytcli.Project{*fixture.articles[0].Project})
		case "/api/admin/projects/0-1/articles":
			if r.URL.Query().Get("$skip") == "0" {
				_ = json.NewEncoder(w).Encode(fixture.articles)
			} else {
				_, _ = w.Write([]byte(`[]`))
			}
		case "/api/articles/1-1":
			_ = json.NewEncoder(w).Encode(fixture.articles[0])
		default:
			t.Errorf("unexpected article read: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := &ytcli.Client{}
	client.SetCredentials(server.URL, "test-token")
	cfg := &config.Config{RenderMarkdown: true}
	app := AppModel{
		client: client, cfg: cfg, state: stateArticles, stateData: "DEMO",
		articles:      newArticlesModel(client, cfg),
		articleDetail: newArticleDetailModel(client, cfg),
		articleForm:   newArticleFormModel(client, cfg),
	}
	model, _ := app.Update(tea.WindowSizeMsg{Width: 100, Height: 38})
	app = model.(AppModel)
	runKnowledgeBaseCommand(t, &app, app.articles.initProject("DEMO", false))
	return app, fixture
}

func articleWorkflowKey(t *testing.T, app *AppModel, key tea.KeyMsg) {
	t.Helper()
	model, cmd := app.Update(key)
	*app = model.(AppModel)
	runKnowledgeBaseCommand(t, app, cmd)
}

func TestArticleWorkflowEditWithExistingEditorAndSaveRetry(t *testing.T) {
	app, fixture := newArticleWorkflowApp(t)
	articleWorkflowKey(t, &app, tea.KeyMsg{Type: tea.KeyEnter})
	articleWorkflowKey(t, &app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if app.state != stateArticleForm || app.articleForm.summaryInput.Value() != "Handbook" {
		t.Fatal("e did not open the populated article form")
	}
	app.articleForm.summaryInput.SetValue("Updated handbook")
	articleWorkflowKey(t, &app, tea.KeyMsg{Type: tea.KeyTab})
	editor, _, _ := fakeExternalEditor(t, "editor")
	t.Setenv("EDITOR", editor+" --wait")
	model, editorCmd := app.Update(tea.KeyMsg{Type: tea.KeyCtrlG})
	app = model.(AppModel)
	if editorCmd == nil {
		t.Fatal("Ctrl+G on content did not open the shared external editor")
	}
	result := runExternalEditor(t, editorCmd)
	model, _ = app.Update(result)
	app = model.(AppModel)
	assertEditorFileRemoved(t, result.tempPath)
	if app.articleForm.contentInput.Value() != os.Getenv("YT_EDITOR_CONTENT") {
		t.Fatal("editor output was not returned to the article draft")
	}
	fixture.mu.Lock()
	if fixture.writes != 0 {
		t.Error("returning from the editor submitted the draft without Ctrl+S")
	}
	fixture.denyWrites = true
	fixture.mu.Unlock()
	articleWorkflowKey(t, &app, tea.KeyMsg{Type: tea.KeyCtrlS})
	if app.state != stateArticleForm || !app.articleForm.errPopupShow || app.articleForm.summaryInput.Value() != "Updated handbook" || app.articleForm.contentInput.Value() != os.Getenv("YT_EDITOR_CONTENT") {
		t.Fatal("failed save lost the form or draft")
	}
	fixture.mu.Lock()
	fixture.denyWrites = false
	fixture.mu.Unlock()
	articleWorkflowKey(t, &app, tea.KeyMsg{Type: tea.KeyEnter}) // Dismiss the existing form error popup.
	articleWorkflowKey(t, &app, tea.KeyMsg{Type: tea.KeyCtrlS})
	if app.state != stateArticleDetail || app.articleDetail.article.Summary != "Updated handbook" || app.articleDetail.article.Content != os.Getenv("YT_EDITOR_CONTENT") {
		t.Fatal("successful save did not return to refreshed article details")
	}
	articleWorkflowKey(t, &app, tea.KeyMsg{Type: tea.KeyEsc})
	if app.state != stateArticles || !strings.Contains(app.View(), "Updated handbook") {
		t.Fatal("article list retained the stale title after editing")
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.writes != 2 {
		t.Fatalf("expected one failed save and one retry, got %d writes", fixture.writes)
	}
}

func TestArticleWorkflowCreateWithOptionalParent(t *testing.T) {
	for _, child := range []bool{false, true} {
		name := "top-level"
		if child {
			name = "child"
		}
		t.Run(name, func(t *testing.T) {
			app, fixture := newArticleWorkflowApp(t)
			articleWorkflowKey(t, &app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
			if app.state != stateArticleForm || app.articleForm.parentSelected {
				t.Fatal("new form should default to no parent")
			}
			articleWorkflowKey(t, &app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("New article")})
			articleWorkflowKey(t, &app, tea.KeyMsg{Type: tea.KeyTab})
			articleWorkflowKey(t, &app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Article body")})
			if child {
				articleWorkflowKey(t, &app, tea.KeyMsg{Type: tea.KeyTab})
				articleWorkflowKey(t, &app, tea.KeyMsg{Type: tea.KeyRight})
			}
			articleWorkflowKey(t, &app, tea.KeyMsg{Type: tea.KeyCtrlS})
			if app.state != stateArticles || !strings.Contains(app.View(), "New article") {
				t.Fatal("creation did not return to the refreshed list")
			}
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			if fixture.writes != 1 || len(fixture.articles) != 2 {
				t.Fatalf("creation should perform one write: writes=%d articles=%d", fixture.writes, len(fixture.articles))
			}
			created := fixture.articles[1]
			if created.Summary != "New article" || created.Content != "Article body" {
				t.Fatalf("created article lost its fields: %+v", created)
			}
			if child && (created.ParentArticle == nil || created.ParentArticle.ID != "1-1") {
				t.Fatal("selected parent did not reach the creation request")
			}
			if !child && created.ParentArticle != nil {
				t.Fatal("top-level creation unexpectedly selected a parent")
			}
		})
	}
}

func TestArticleWorkflowCancelAndIgnoreLateSave(t *testing.T) {
	app, fixture := newArticleWorkflowApp(t)
	articleWorkflowKey(t, &app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	oldRequest := app.articleForm.requestID
	articleWorkflowKey(t, &app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("qS? unsaved draft")})
	if app.searchShow || app.helpShow {
		t.Fatal("global shortcuts intercepted article form input")
	}
	articleWorkflowKey(t, &app, tea.KeyMsg{Type: tea.KeyEsc})
	if app.state != stateArticles {
		t.Fatal("Esc did not cancel article creation")
	}
	articleWorkflowKey(t, &app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	model, cmd := app.Update(articleSavedMsg{requestID: oldRequest, projectCode: "DEMO"})
	app = model.(AppModel)
	if app.state != stateArticleForm || cmd != nil {
		t.Fatal("late completion navigated away from a newer form")
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.writes != 0 {
		t.Fatalf("cancelled draft performed %d writes", fixture.writes)
	}
}

func TestArticleWorkflowTitleOnlyEditPreservesBody(t *testing.T) {
	app, fixture := newArticleWorkflowApp(t)
	const original = "# Example\r\n\r\n```go\r\n\treturn value\r\n```\r\n"
	fixture.mu.Lock()
	fixture.articles[0].Content = original
	fixture.mu.Unlock()
	articleWorkflowKey(t, &app, tea.KeyMsg{Type: tea.KeyEnter})
	articleWorkflowKey(t, &app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	app.articleForm.summaryInput.SetValue("Renamed handbook")
	articleWorkflowKey(t, &app, tea.KeyMsg{Type: tea.KeyCtrlS})
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.articles[0].Content != original {
		t.Fatalf("title-only edit rewrote article content: got %q, want %q", fixture.articles[0].Content, original)
	}
}
