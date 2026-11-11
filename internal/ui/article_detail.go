package ui

import (
	"fmt"
	"strings"
	"time"
	"yt-tui/internal/config"
	"yt-tui/internal/ytcli"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type articleDetailModel struct {
	client         *ytcli.Client
	articleID      string
	article        *ytcli.Article
	requestID      uint64
	loading        bool
	err            error
	renderMarkdown bool
	spinner        spinner.Model
	viewport       viewport.Model
	width          int
	height         int
}

type articleDetailDataMsg struct {
	articleID string
	requestID uint64
	article   *ytcli.Article
	err       error
}

func newArticleDetailModel(client *ytcli.Client, cfg *config.Config) articleDetailModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorViolet))
	renderMarkdown := true
	if cfg != nil {
		renderMarkdown = cfg.RenderMarkdown
	}
	return articleDetailModel{
		client:         client,
		renderMarkdown: renderMarkdown,
		spinner:        s,
		viewport:       viewport.New(1, 1),
	}
}

func (m *articleDetailModel) initArticle(id string) tea.Cmd {
	m.articleID = id
	m.article = nil
	m.viewport.GotoTop()
	m.updateViewportSizes()
	return m.loadArticleCmd()
}

func (m *articleDetailModel) loadArticleCmd() tea.Cmd {
	m.requestID++
	m.loading = true
	m.err = nil
	id, requestID, client := m.articleID, m.requestID, m.client
	load := func() tea.Msg {
		if client == nil {
			return articleDetailDataMsg{articleID: id, requestID: requestID, err: fmt.Errorf("YouTrack client is not configured")}
		}
		article, err := client.GetArticle(id)
		return articleDetailDataMsg{articleID: id, requestID: requestID, article: article, err: err}
	}
	return tea.Batch(m.spinner.Tick, load)
}

func (m articleDetailModel) Update(msg tea.Msg) (articleDetailModel, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	case articleDetailDataMsg:
		if msg.articleID != m.articleID || msg.requestID != m.requestID {
			return m, nil
		}
		m.loading = false
		m.err = msg.err
		if m.err == nil && msg.article == nil {
			m.err = fmt.Errorf("YouTrack did not return an article")
		}
		if m.err == nil {
			m.article = msg.article
		}
		m.updateViewportSizes()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "backspace":
			return m, func() tea.Msg { return popStateMsg{} }
		case "r":
			cmd := m.loadArticleCmd()
			return m, cmd
		case "m":
			m.renderMarkdown = !m.renderMarkdown
			m.updateViewportSizes()
			return m, nil
		}
		if m.loading || m.err != nil || m.article == nil {
			return m, nil
		}
		switch msg.String() {
		case "e":
			data := "edit:" + m.articleID
			return m, func() tea.Msg { return pushStateMsg{state: stateArticleForm, data: data} }
		case "home", "g":
			m.viewport.GotoTop()
			return m, nil
		case "end", "G":
			m.viewport.GotoBottom()
			return m, nil
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m articleDetailModel) header() string {
	width := max(1, m.width)
	title := "Knowledge Base"
	metadata := m.articleID
	if m.article != nil {
		title = m.article.Summary
		if strings.TrimSpace(title) == "" {
			title = "Untitled article"
		}
		if m.article.IDReadable != "" {
			metadata = m.article.IDReadable
		}
		if m.article.Project != nil {
			project := m.article.Project.ShortName
			if project == "" {
				project = m.article.Project.Name
			}
			if project != "" {
				metadata += "  ·  " + project
			}
		}
		if m.article.Updated > 0 {
			metadata += "  ·  Updated " + time.UnixMilli(m.article.Updated).Format("2006-01-02 15:04")
		}
	}
	// Titles and project names are single-line labels, including when supplied
	// by a server that allows newlines or terminal escape sequences in them.
	oneLine := func(value string) string { return strings.Join(strings.Fields(ansi.Strip(value)), " ") }
	title = ansi.Truncate(oneLine(title), width, "…")
	metadata = ansi.Truncate(oneLine(metadata), width, "…")
	return lipgloss.NewStyle().Foreground(lipgloss.Color(ColorViolet)).Bold(true).Render(title) + "\n" + StyleSubtext.Render(metadata)
}

func (m articleDetailModel) footer() string {
	toggle := "Markdown"
	if m.renderMarkdown {
		toggle = "Plain text"
	}
	help := "[Esc] Back  [↑↓/jk] Scroll  [PgUp/PgDn] Page  [e] Edit  [m] " + toggle + "  [r] Refresh  [?] Help  " + quitHelp()
	return StyleHelp.Render(wrapFooterHelp(help, max(1, m.width)))
}

func (m *articleDetailModel) updateViewportSizes() {
	width := max(1, m.width)
	height := max(1, m.height)
	m.viewport.Width = width
	m.viewport.Height = max(1, height-lipgloss.Height(m.header())-lipgloss.Height(m.footer())-2)
	if m.article == nil {
		m.viewport.SetContent("")
		return
	}

	content := ansi.Strip(m.article.Content)
	if strings.TrimSpace(content) == "" {
		content = "This article has no content."
	} else if m.renderMarkdown {
		renderer, err := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(width))
		if err == nil {
			if rendered, err := renderer.Render(content); err == nil {
				content = strings.Trim(rendered, "\n")
			}
		}
	}
	// Hard wrapping also keeps long URLs and fenced code inside narrow floats.
	content = ansi.Hardwrap(content, width, true)
	offset := m.viewport.YOffset
	m.viewport.SetContent(content)
	m.viewport.SetYOffset(offset)
}

func (m articleDetailModel) View() string {
	width, height := max(1, m.width), max(1, m.height)
	body := m.viewport.View()
	if m.loading {
		body = lipgloss.Place(width, m.viewport.Height, lipgloss.Center, lipgloss.Center, m.spinner.View()+" Loading article...")
	} else if m.err != nil {
		body = lipgloss.NewStyle().Width(width).MaxHeight(m.viewport.Height).
			Render(StyleErrorMessage.Render("Could not load article: " + ansi.Strip(m.err.Error())))
	} else if m.article == nil {
		body = StyleSubtext.Render("No article selected.")
	}
	view := lipgloss.JoinVertical(lipgloss.Left, m.header(), "", body, "", m.footer())
	return lipgloss.NewStyle().MaxWidth(width).MaxHeight(height).Render(view)
}
