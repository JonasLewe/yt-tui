package ui

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"yt-tui/internal/ytcli"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func articlesTestModel(articles []ytcli.Article) articlesModel {
	m := newArticlesModel(nil, nil)
	m.width, m.height = 100, 18
	m.projectCode = "DEMO"
	m, _ = m.Update(articlesDataMsg{projectCode: "DEMO", articles: articles})
	return m
}

func articleTestKey(key string) tea.KeyMsg {
	switch key {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "pgdown":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	case "home":
		return tea.KeyMsg{Type: tea.KeyHome}
	case "end":
		return tea.KeyMsg{Type: tea.KeyEnd}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
}

func TestArticlesHierarchyKeepsSiblingOrderAndNestedArticles(t *testing.T) {
	articles := []ytcli.Article{
		{ID: "child-later", Summary: "Alpha child", Ordinal: 2, ParentArticle: &ytcli.Article{ID: "root"}},
		{ID: "second-root", Summary: "Another root", Ordinal: 3},
		{ID: "grandchild", Summary: "Nested article", ParentArticle: &ytcli.Article{ID: "child-first"}},
		{ID: "root", Summary: "Zebra root", Ordinal: 1},
		{ID: "child-first", Summary: "Zebra child", Ordinal: 1, ParentArticle: &ytcli.Article{ID: "root"}},
	}
	rows := buildArticleRows(articles, "")
	var ids []string
	var depths []int
	for _, row := range rows {
		ids = append(ids, row.article.ID)
		depths = append(depths, row.depth)
	}
	if want := []string{"root", "child-first", "grandchild", "child-later", "second-root"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("article hierarchy = %v, want %v", ids, want)
	}
	if want := []int{0, 1, 2, 1, 0}; !reflect.DeepEqual(depths, want) {
		t.Fatalf("hierarchy indentation = %v, want %v", depths, want)
	}
}

func TestArticlesHierarchyKeepsOrphansAndCyclesReachable(t *testing.T) {
	articles := []ytcli.Article{
		{ID: "orphan", ParentArticle: &ytcli.Article{ID: "not-visible"}},
		{ID: "self", ParentArticle: &ytcli.Article{ID: "self"}},
		{ID: "cycle-a", ParentArticle: &ytcli.Article{ID: "cycle-b"}},
		{ID: "cycle-b", ParentArticle: &ytcli.Article{ID: "cycle-a"}},
		{ID: "orphan", ParentArticle: &ytcli.Article{ID: "not-visible"}},
	}
	rows := buildArticleRows(articles, "")
	seen := map[string]bool{}
	for _, row := range rows {
		if seen[row.article.ID] {
			t.Fatalf("article %q appears more than once", row.article.ID)
		}
		seen[row.article.ID] = true
	}
	if len(seen) != 4 {
		t.Fatalf("an article became unreachable: %v", seen)
	}
}

func TestArticlesFilterFindsNestedTitlesAndIDs(t *testing.T) {
	articles := []ytcli.Article{
		{ID: "root", IDReadable: "DEMO-A-1", Summary: "Getting started"},
		{ID: "nested", IDReadable: "DEMO-A-2", Summary: "VPN Access", ParentArticle: &ytcli.Article{ID: "root"}},
	}
	for _, query := range []string{"vpn", "demo-a-2", "  ACCESS  "} {
		t.Run(query, func(t *testing.T) {
			rows := buildArticleRows(articles, query)
			if len(rows) != 1 || rows[0].article.ID != "nested" || rows[0].depth != 0 {
				t.Fatalf("nested match not independently visible: %+v", rows)
			}
		})
	}
}

func TestArticlesSearchKeysRemainInputAndEscapeRestoresList(t *testing.T) {
	m := articlesTestModel([]ytcli.Article{{ID: "1", Summary: "qS? reference"}})
	m, _ = m.Update(articleTestKey("/"))
	if !m.searchMode || !m.searchInput.Focused() {
		t.Fatal("filter did not gain input focus")
	}
	for _, value := range []string{"q", "S", "?"} {
		m, _ = m.Update(articleTestKey(value))
	}
	if m.searchInput.Value() != "qS?" || len(m.rows) != 1 {
		t.Fatalf("filter input was intercepted: %q", m.searchInput.Value())
	}
	m, cmd := m.Update(articleTestKey("enter"))
	if m.searchMode || cmd != nil || m.searchInput.Focused() {
		t.Fatal("Enter should finish filtering without opening an article")
	}
	m, cmd = m.Update(articleTestKey("esc"))
	if m.searchInput.Value() != "" || cmd != nil {
		t.Fatal("first Escape should clear the active filter")
	}
	_, cmd = m.Update(articleTestKey("esc"))
	if cmd == nil {
		t.Fatal("Escape with no filter should navigate back")
	}
	if _, ok := cmd().(popStateMsg); !ok {
		t.Fatalf("unexpected back message %T", cmd())
	}
}

func TestArticlesOpenSelectedArticleAndPreserveBackState(t *testing.T) {
	m := articlesTestModel([]ytcli.Article{
		{ID: "first", Summary: "Article one"},
		{ID: "second", Summary: "Article two"},
	})
	m, _ = m.Update(articleTestKey("down"))
	m, cmd := m.Update(articleTestKey("enter"))
	if cmd == nil {
		t.Fatal("Enter did not request the article reader")
	}
	msg, ok := cmd().(pushStateMsg)
	if !ok || msg.state != stateArticleDetail || msg.data != "second" {
		t.Fatalf("article navigation = %+v", msg)
	}
	m.searchInput.SetValue("Article")
	if cmd := m.initProject("DEMO", true); cmd != nil {
		t.Fatal("returning from the reader should retain the loaded list")
	}
	if m.cursor != 1 || m.searchInput.Value() != "Article" {
		t.Fatal("returning from the reader lost selection or filter")
	}
	if cmd := m.initProject("OTHER", false); cmd == nil {
		t.Fatal("a different project must load its articles")
	}
	if len(m.rows) != 0 || m.searchInput.Value() != "" || m.cursor != 0 {
		t.Fatal("a different project retained previous project state")
	}
}

func TestArticlesIgnoreStaleLoadsAndAllowErrorRecovery(t *testing.T) {
	m := articlesTestModel([]ytcli.Article{{ID: "old", Summary: "Existing article"}})
	m, cmd := m.Update(articleTestKey("r"))
	if !m.loading || cmd == nil || m.requestID == 0 {
		t.Fatal("refresh did not start a new request")
	}
	requestID := m.requestID
	stale := []articlesDataMsg{
		{projectCode: "DEMO", requestID: requestID - 1, articles: []ytcli.Article{{ID: "stale"}}},
		{projectCode: "OTHER", requestID: requestID, articles: []ytcli.Article{{ID: "wrong-project"}}},
		{projectCode: "DEMO", serverURL: "https://other.invalid", requestID: requestID, articles: []ytcli.Article{{ID: "wrong-server"}}},
	}
	for _, msg := range stale {
		m, _ = m.Update(msg)
		if !m.loading || m.rows[0].article.ID != "old" {
			t.Fatal("stale response changed the current project")
		}
	}
	m, _ = m.Update(articlesDataMsg{projectCode: "DEMO", requestID: requestID, err: errors.New("access denied")})
	if m.loading || !strings.Contains(m.View(), "access denied") {
		t.Fatal("request error did not render")
	}
	_, back := m.Update(articleTestKey("esc"))
	if back == nil {
		t.Fatal("an error trapped the user in the article list")
	}
	m, cmd = m.Update(articleTestKey("r"))
	if cmd == nil || !m.loading || m.err != nil {
		t.Fatal("refresh did not recover from an error")
	}
}

func TestArticlesNavigationAndSmallTerminalBounds(t *testing.T) {
	var articles []ytcli.Article
	for _, id := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		articles = append(articles, ytcli.Article{ID: id, Summary: strings.Repeat("Long title 知識 ", 10)})
	}
	m := articlesTestModel(articles)
	m.height = 10
	m, _ = m.Update(articleTestKey("end"))
	if m.cursor != 9 || m.offset == 0 {
		t.Fatal("End did not scroll to the final article")
	}
	m, _ = m.Update(articleTestKey("home"))
	m, _ = m.Update(articleTestKey("pgdown"))
	if m.cursor == 0 {
		t.Fatal("page navigation did not move")
	}
	for _, size := range [][2]int{{80, 20}, {20, 8}, {4, 3}, {1, 1}, {0, 0}} {
		m.width, m.height = size[0], size[1]
		view := m.View()
		if lipgloss.Width(view) > max(1, size[0]) || lipgloss.Height(view) > max(1, size[1]) {
			t.Fatalf("view exceeds dimensions %v: width=%d, height=%d", size, lipgloss.Width(view), lipgloss.Height(view))
		}
	}
}

func TestArticlesEmptyStateAllowsBackWhileLoading(t *testing.T) {
	m := articlesTestModel(nil)
	if !strings.Contains(m.View(), "No articles visible") {
		t.Fatal("empty project message is missing")
	}
	m.loading = true
	_, cmd := m.Update(articleTestKey("esc"))
	if cmd == nil {
		t.Fatal("loading prevents back navigation")
	}
}
