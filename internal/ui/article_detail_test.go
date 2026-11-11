package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"yt-tui/internal/config"
	"yt-tui/internal/ytcli"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func loadedArticleReader(content string, markdown bool) articleDetailModel {
	m := newArticleDetailModel(nil, &config.Config{RenderMarkdown: markdown})
	m.width, m.height = 80, 24
	m.initArticle("226-1")
	m, _ = m.Update(articleDetailDataMsg{
		articleID: m.articleID,
		requestID: m.requestID,
		article: &ytcli.Article{
			ID: "226-1", IDReadable: "DEMO-A-1", Summary: "Getting started", Content: content,
			Project: &ytcli.Project{ShortName: "DEMO"},
		},
	})
	return m
}

func TestArticleDetailMarkdownToggle(t *testing.T) {
	cfg := &config.Config{RenderMarkdown: true}
	m := newArticleDetailModel(nil, cfg)
	m.width, m.height = 80, 24
	m.initArticle("226-1")
	content := "# Welcome\n\nRead **these instructions**."
	m, _ = m.Update(articleDetailDataMsg{
		articleID: m.articleID, requestID: m.requestID,
		article: &ytcli.Article{ID: "226-1", IDReadable: "DEMO-A-1", Summary: "Introduction", Content: content},
	})
	rendered := ansi.Strip(m.View())
	if !strings.Contains(rendered, "Welcome") || !strings.Contains(rendered, "these instructions") {
		t.Fatalf("rendered Markdown is missing content: %q", rendered)
	}
	if strings.Contains(rendered, "**these instructions**") {
		t.Fatal("Markdown emphasis was not rendered")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	plain := ansi.Strip(m.View())
	if !strings.Contains(plain, "# Welcome") || !strings.Contains(plain, "**these instructions**") {
		t.Fatalf("plain text toggle lost Markdown source: %q", plain)
	}
	if !cfg.RenderMarkdown {
		t.Fatal("article toggle changed the shared configuration")
	}
	if m.article.Content != content {
		t.Fatal("rendering mutated the article content")
	}
}

func TestArticleDetailScrollAndResize(t *testing.T) {
	var content strings.Builder
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&content, "Article line %03d\n", i)
	}
	m := loadedArticleReader(content.String(), false)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.viewport.YOffset != 1 {
		t.Fatalf("j did not scroll one line: %d", m.viewport.YOffset)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if m.viewport.YOffset <= 1 {
		t.Fatal("PageDown did not move through the article")
	}
	offset := m.viewport.YOffset
	m.width, m.height = 45, 18
	m.updateViewportSizes()
	if m.viewport.YOffset != offset {
		t.Fatalf("resize reset scroll position: got %d, want %d", m.viewport.YOffset, offset)
	}
	if !strings.Contains(ansi.Strip(m.View()), fmt.Sprintf("Article line %03d", offset)) {
		t.Fatal("resized viewport lost the visible article content")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if !m.viewport.AtBottom() {
		t.Fatal("End did not reach the article bottom")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if !m.viewport.AtTop() {
		t.Fatal("g did not reach the article top")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if m.viewport.YOffset == 0 {
		t.Fatal("Ctrl+D did not scroll")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	if !m.viewport.AtTop() {
		t.Fatal("Ctrl+U did not return to the top")
	}
}

func TestArticleDetailFitsNarrowTerminals(t *testing.T) {
	m := loadedArticleReader("# Heading\n\n"+strings.Repeat("https://example.test/path/", 30), true)
	m.article.Summary = strings.Repeat("Long title 界", 20)
	for _, dimensions := range [][2]int{{0, 0}, {1, 1}, {8, 4}, {24, 12}, {80, 24}} {
		m.width, m.height = dimensions[0], dimensions[1]
		m.updateViewportSizes()
		view := m.View()
		if got := lipgloss.Width(view); got > max(1, m.width) {
			t.Errorf("%dx%d reader exceeds width: %d", m.width, m.height, got)
		}
		if got := lipgloss.Height(view); got > max(1, m.height) {
			t.Errorf("%dx%d reader exceeds height: %d", m.width, m.height, got)
		}
		if m.viewport.Width < 1 || m.viewport.Height < 1 {
			t.Fatal("viewport dimensions became nonpositive")
		}
	}
}

func TestArticleDetailEmptyContentAndMetadata(t *testing.T) {
	m := loadedArticleReader("\n\t", true)
	view := ansi.Strip(m.View())
	for _, want := range []string{"Getting started", "DEMO-A-1", "DEMO", "This article has no content."} {
		if !strings.Contains(view, want) {
			t.Errorf("reader missing %q: %q", want, view)
		}
	}
}

func TestArticleDetailRejectsStaleLoads(t *testing.T) {
	m := loadedArticleReader("Current content", false)
	oldRequest := m.requestID
	m.initArticle("226-2")
	stale := &ytcli.Article{ID: "226-1", Content: "Stale content"}
	m, _ = m.Update(articleDetailDataMsg{articleID: "226-1", requestID: oldRequest, article: stale})
	if m.article != nil || !m.loading {
		t.Fatal("another article's late response replaced the pending article")
	}
	m, _ = m.Update(articleDetailDataMsg{articleID: "226-2", requestID: oldRequest, article: stale})
	if m.article != nil || !m.loading {
		t.Fatal("an earlier request's response replaced the pending article")
	}
	m, _ = m.Update(articleDetailDataMsg{
		articleID: "226-2", requestID: m.requestID,
		article: &ytcli.Article{ID: "226-2", Content: "New content"},
	})
	if m.loading || m.article == nil || m.article.ID != "226-2" {
		t.Fatal("current article response was not accepted")
	}
}

func TestArticleDetailBackAndRetryDuringErrorOrLoad(t *testing.T) {
	m := newArticleDetailModel(nil, nil)
	m.width, m.height = 80, 24
	m.initArticle("226-1")
	for _, key := range []tea.KeyType{tea.KeyEsc, tea.KeyBackspace} {
		_, cmd := m.Update(tea.KeyMsg{Type: key})
		if cmd == nil {
			t.Fatal("back key did not work while loading")
		}
		if _, ok := cmd().(popStateMsg); !ok {
			t.Fatal("back key did not emit a navigation message")
		}
	}
	m, _ = m.Update(articleDetailDataMsg{articleID: m.articleID, requestID: m.requestID, err: errors.New("permission denied")})
	if m.loading || !strings.Contains(ansi.Strip(m.View()), "permission denied") {
		t.Fatal("request error was not presented")
	}
	m.err = errors.New(strings.Repeat("Long server error. ", 200))
	if !strings.Contains(ansi.Strip(m.View()), "[r] Refresh") {
		t.Fatal("long error pushed recovery shortcuts offscreen")
	}
	oldRequest := m.requestID
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd == nil || !m.loading || m.err != nil || m.requestID <= oldRequest {
		t.Fatal("refresh did not start a new request after an error")
	}
	m, _ = m.Update(articleDetailDataMsg{articleID: m.articleID, requestID: m.requestID})
	if m.err == nil || m.loading {
		t.Fatal("empty API response should yield a recoverable error")
	}
}
