package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"yt-tui/internal/config"
	"yt-tui/internal/ytcli"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestKnowledgeBaseProjectShortcuts(t *testing.T) {
	key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("K")}
	for _, code := range []string{"DEMO", "ME", ""} {
		t.Run("dashboard/"+code, func(t *testing.T) {
			m := newDashboardModel(nil, nil)
			m.loadingIssues, m.loadingProj = false, false
			m.active = panelProjects
			m.projects = []ytcli.Project{{ShortName: code}}
			_, cmd := m.Update(key)
			assertKnowledgeBaseShortcut(t, cmd, code)
		})
		t.Run("projects/"+code, func(t *testing.T) {
			m := newProjectsModel(nil)
			m.height = 30
			m.loading = false
			m.table.SetRows([]table.Row{{code, "Project", ""}})
			_, cmd := m.Update(key)
			assertKnowledgeBaseShortcut(t, cmd, code)
		})
		t.Run("issues/"+code, func(t *testing.T) {
			m := newIssuesModel(nil, nil)
			m.projectCode, m.loading = code, false
			_, cmd := m.Update(key)
			assertKnowledgeBaseShortcut(t, cmd, code)
		})
	}
}

func assertKnowledgeBaseShortcut(t *testing.T, cmd tea.Cmd, project string) {
	t.Helper()
	if project == "" || project == "ME" {
		if cmd != nil {
			if msg, ok := cmd().(pushStateMsg); ok && msg.state == stateArticles {
				t.Fatal("non-project view opened the Knowledge Base")
			}
		}
		return
	}
	if cmd == nil {
		t.Fatal("Knowledge Base shortcut returned no navigation command")
	}
	msg, ok := cmd().(pushStateMsg)
	if !ok || msg.state != stateArticles || msg.data != project {
		t.Fatalf("unexpected navigation: %#v", msg)
	}
}

// Run only finite navigation/data commands. Spinner and cursor timers do not
// participate in the assertions and must not keep this deterministic test alive.
func runKnowledgeBaseCommand(t *testing.T, app *AppModel, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, child := range batch {
			runKnowledgeBaseCommand(t, app, child)
		}
		return
	}
	switch msg.(type) {
	case pushStateMsg, popStateMsg, articlesDataMsg, articleDetailDataMsg:
		updated, next := app.Update(msg)
		*app = updated.(AppModel)
		runKnowledgeBaseCommand(t, app, next)
	}
}

func TestAppKnowledgeBaseBrowseReadAndReturn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Knowledge Base made a write request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/admin/projects":
			fmt.Fprint(w, `[{"id":"0-1","shortName":"DEMO","name":"Demo"}]`)
		case "/api/admin/projects/0-1/articles":
			if r.URL.Query().Get("$skip") == "0" {
				fmt.Fprint(w, `[{"id":"1-1","idReadable":"DEMO-A-1","summary":"Handbook","ordinal":0},{"id":"1-2","idReadable":"DEMO-A-2","summary":"Field guide","parentArticle":{"id":"1-1"},"ordinal":1}]`)
			} else {
				fmt.Fprint(w, `[]`)
			}
		case "/api/articles/1-2":
			fmt.Fprint(w, `{"id":"1-2","idReadable":"DEMO-A-2","summary":"Field guide","content":"# Run checks\n\nUse the project handbook.","project":{"shortName":"DEMO"}}`)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &ytcli.Client{}
	client.SetCredentials(server.URL, "test-token")
	cfg := &config.Config{RenderMarkdown: true}
	app := AppModel{
		client: client, cfg: cfg, state: stateProjects,
		projects:      newProjectsModel(client),
		articles:      newArticlesModel(client, cfg),
		articleDetail: newArticleDetailModel(client, cfg),
	}
	model, _ := app.Update(tea.WindowSizeMsg{Width: 100, Height: 32})
	app = model.(AppModel)
	app.projects.loading = false
	app.projects.table.SetRows([]table.Row{{"DEMO", "Demo", ""}})
	model, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("K")})
	app = model.(AppModel)
	runKnowledgeBaseCommand(t, &app, cmd)
	if app.state != stateArticles || !strings.Contains(app.View(), "Field guide") {
		t.Fatalf("project articles not displayed: state=%s view=%s", app.state, app.View())
	}
	// Filter to a nested article, select it, and ensure back preserves context.
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("/")},
		{Type: tea.KeyRunes, Runes: []rune("Field")},
		{Type: tea.KeyEnter},
		{Type: tea.KeyEnter},
	} {
		model, cmd = app.Update(key)
		app = model.(AppModel)
		runKnowledgeBaseCommand(t, &app, cmd)
	}
	if app.state != stateArticleDetail || !strings.Contains(ansi.Strip(app.View()), "Run checks") {
		t.Fatalf("article reader not displayed: state=%s view=%s", app.state, app.View())
	}
	model, _ = app.Update(tea.WindowSizeMsg{Width: 50, Height: 18})
	app = model.(AppModel)
	if w, h := lipgloss.Width(app.View()), lipgloss.Height(app.View()); w > 50 || h > 18 {
		t.Fatalf("article view exceeds resized terminal: %dx%d", w, h)
	}
	model, cmd = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = model.(AppModel)
	runKnowledgeBaseCommand(t, &app, cmd)
	if app.state != stateArticles || app.articles.searchInput.Value() != "Field" || app.articles.selectedID() != "1-2" {
		t.Fatal("back navigation lost the article list selection/filter")
	}
	if len(app.history) != 1 || app.history[0].state != stateProjects {
		t.Fatalf("unexpected navigation history: %#v", app.history)
	}
}

func TestAppKnowledgeBaseFilterKeepsGlobalKeys(t *testing.T) {
	app := AppModel{state: stateArticles, articles: newArticlesModel(nil, nil)}
	app.articles.width, app.articles.height = 80, 25
	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	app = model.(AppModel)
	for _, key := range []string{"q", "S", "?", "K"} {
		model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		app = model.(AppModel)
	}
	if app.searchShow || app.helpShow || app.articles.searchInput.Value() != "qS?K" {
		t.Fatal("global hotkeys intercepted article filter input")
	}
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = model.(AppModel)
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("q did not quit after leaving filter input")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q did not emit tea.QuitMsg")
	}
}

func TestAppKnowledgeBaseBreadcrumbFitsNarrowTerminal(t *testing.T) {
	app := AppModel{
		state: stateArticleDetail,
		history: []navEntry{
			{state: stateDashboard},
			{state: stateProjects},
			{state: stateIssues, data: "DEMO"},
			{state: stateArticles, data: "DEMO"},
		},
		articles:      newArticlesModel(nil, nil),
		articleDetail: newArticleDetailModel(nil, nil),
	}
	app.articleDetail.article = &ytcli.Article{ID: "1-1", Summary: "Handbook", Content: "Example article"}
	for _, size := range [][2]int{{50, 18}, {32, 14}} {
		model, _ := app.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		app = model.(AppModel)
		view := app.View()
		if width, height := lipgloss.Width(view), lipgloss.Height(view); width > size[0] || height > size[1] {
			t.Errorf("article with long breadcrumb exceeds %dx%d terminal: %dx%d", size[0], size[1], width, height)
		}
	}
}
