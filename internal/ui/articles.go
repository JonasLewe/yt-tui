package ui

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"yt-tui/internal/config"
	"yt-tui/internal/ytcli"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type articleRow struct {
	article ytcli.Article
	depth   int
}

type articlesModel struct {
	client      *ytcli.Client
	projectCode string
	serverURL   string
	articles    []ytcli.Article
	rows        []articleRow
	cursor      int
	offset      int
	searchInput textinput.Model
	searchMode  bool
	loading     bool
	loaded      bool
	err         error
	requestID   uint64
	spinner     spinner.Model
	width       int
	height      int
}

type articlesDataMsg struct {
	projectCode string
	serverURL   string
	requestID   uint64
	articles    []ytcli.Article
	err         error
}

func newArticlesModel(client *ytcli.Client, _ *config.Config) articlesModel {
	input := textinput.New()
	input.Prompt = "/ "
	input.Placeholder = "Filter article title or ID..."
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorViolet))
	return articlesModel{client: client, searchInput: input, spinner: s}
}

func (m *articlesModel) initProject(projectCode string, isBack bool) tea.Cmd {
	serverURL := ""
	if m.client != nil {
		serverURL = m.client.GetConfiguredBaseURL()
	}
	sameProject := m.projectCode == projectCode && m.serverURL == serverURL
	if isBack && sameProject && m.loaded && !m.loading {
		m.ensureCursorVisible()
		return nil
	}
	if !isBack || !sameProject {
		m.articles = nil
		m.rows = nil
		m.cursor = 0
		m.offset = 0
		m.searchInput.SetValue("")
		m.searchInput.Blur()
		m.searchMode = false
		m.loaded = false
	}
	m.projectCode = projectCode
	m.serverURL = serverURL
	return m.startLoading()
}

func (m *articlesModel) startLoading() tea.Cmd {
	m.requestID++
	m.loading = true
	m.err = nil
	projectCode, serverURL, requestID := m.projectCode, m.serverURL, m.requestID
	client := m.client
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		msg := articlesDataMsg{projectCode: projectCode, serverURL: serverURL, requestID: requestID}
		if client == nil {
			msg.err = fmt.Errorf("YouTrack client is unavailable")
		} else {
			msg.articles, msg.err = client.ListProjectArticles(projectCode)
		}
		return msg
	})
}

func (m *articlesModel) invalidateProject(projectCode string) {
	if m.projectCode == projectCode {
		m.loaded = false
		m.requestID++
	}
}

func (m articlesModel) Update(msg tea.Msg) (articlesModel, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	case articlesDataMsg:
		if msg.projectCode != m.projectCode || msg.serverURL != m.serverURL || msg.requestID != m.requestID {
			return m, nil
		}
		if m.client != nil && m.client.GetConfiguredBaseURL() != m.serverURL {
			return m, nil
		}
		m.loading = false
		m.err = msg.err
		m.loaded = msg.err == nil
		if msg.err == nil {
			selected := m.selectedID()
			m.articles = msg.articles
			m.rebuildRows(selected)
		}
	case tea.KeyMsg:
		if m.searchMode {
			switch msg.String() {
			case "esc", "enter":
				m.searchMode = false
				m.searchInput.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			m.cursor, m.offset = 0, 0
			m.rebuildRows("")
			return m, cmd
		}
		switch msg.String() {
		case "esc", "backspace":
			if m.searchInput.Value() != "" {
				m.searchInput.SetValue("")
				m.rebuildRows("")
				return m, nil
			}
			return m, func() tea.Msg { return popStateMsg{} }
		case "r":
			cmd := m.startLoading()
			return m, cmd
		case "/":
			m.searchMode = true
			m.searchInput.Width = max(1, m.width-2)
			cmd := m.searchInput.Focus()
			return m, cmd
		}
		if m.loading || m.err != nil {
			return m, nil
		}
		switch msg.String() {
		case "n":
			data := "new:" + m.projectCode + ":" + m.selectedID()
			return m, func() tea.Msg { return pushStateMsg{state: stateArticleForm, data: data} }
		case "up", "k", "ctrl+p":
			m.cursor--
		case "down", "j", "ctrl+n":
			m.cursor++
		case "pgup", "ctrl+u":
			m.cursor -= max(1, m.listHeight())
		case "pgdown", "ctrl+d":
			m.cursor += max(1, m.listHeight())
		case "home", "g":
			m.cursor = 0
		case "end", "G":
			m.cursor = len(m.rows) - 1
		case "enter":
			if id := m.selectedID(); id != "" {
				return m, func() tea.Msg { return pushStateMsg{state: stateArticleDetail, data: id} }
			}
		}
	}
	m.searchInput.Width = max(1, m.width-2)
	m.ensureCursorVisible()
	return m, nil
}

func articleRowID(article ytcli.Article) string {
	if article.ID != "" {
		return article.ID
	}
	return article.IDReadable
}

func (m articlesModel) selectedID() string {
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		return articleRowID(m.rows[m.cursor].article)
	}
	return ""
}

// buildArticleRows keeps orphaned articles visible and visits each article once,
// including malformed cycles. Filtering searches the complete project tree.
func buildArticleRows(articles []ytcli.Article, filter string) []articleRow {
	nodes := make(map[string]ytcli.Article, len(articles))
	var ids []string
	for _, article := range articles {
		id := articleRowID(article)
		if id == "" {
			continue
		}
		if _, exists := nodes[id]; !exists {
			nodes[id] = article
			ids = append(ids, id)
		}
	}
	sort.SliceStable(ids, func(i, j int) bool {
		a, b := nodes[ids[i]], nodes[ids[j]]
		if a.Ordinal != b.Ordinal {
			return a.Ordinal < b.Ordinal
		}
		if as, bs := strings.ToLower(a.Summary), strings.ToLower(b.Summary); as != bs {
			return as < bs
		}
		return ids[i] < ids[j]
	})
	children := make(map[string][]string)
	var roots []string
	for _, id := range ids {
		parentID := ""
		if parent := nodes[id].ParentArticle; parent != nil {
			parentID = articleRowID(*parent)
		}
		if _, found := nodes[parentID]; !found || parentID == id {
			roots = append(roots, id)
		} else {
			children[parentID] = append(children[parentID], id)
		}
	}
	query := strings.ToLower(strings.TrimSpace(filter))
	visited := make(map[string]bool, len(nodes))
	var rows []articleRow
	var visit func(string, int)
	visit = func(id string, depth int) {
		if visited[id] {
			return
		}
		visited[id] = true
		article := nodes[id]
		if query == "" {
			rows = append(rows, articleRow{article: article, depth: depth})
		} else if strings.Contains(strings.ToLower(article.Summary), query) ||
			strings.Contains(strings.ToLower(article.IDReadable), query) ||
			strings.Contains(strings.ToLower(article.ID), query) {
			rows = append(rows, articleRow{article: article})
		}
		for _, child := range children[id] {
			visit(child, depth+1)
		}
	}
	for _, id := range roots {
		visit(id, 0)
	}
	for _, id := range ids {
		visit(id, 0)
	}
	return rows
}

func (m *articlesModel) rebuildRows(selected string) {
	m.rows = buildArticleRows(m.articles, m.searchInput.Value())
	if selected != "" {
		for i, row := range m.rows {
			if articleRowID(row.article) == selected {
				m.cursor = i
				break
			}
		}
	}
	m.ensureCursorVisible()
}

func (m articlesModel) footerLines() []string {
	help := "[Esc] Back  [↑↓/jk] Navigate  [Enter] Read  [n] New  [/] Filter  [r] Refresh  [?] Help  [q] Quit"
	if m.searchMode {
		help = "[Enter/Esc] Finish filter  Type an article title or ID"
	}
	lines := strings.Split(wrapFooterHelp(help, max(1, m.width)), "\n")
	return lines[:min(len(lines), max(0, m.height-4))]
}

func (m articlesModel) listHeight() int {
	footerHeight := len(m.footerLines())
	gap := 0
	if footerHeight > 0 {
		gap = 1
	}
	return max(0, m.height-2-footerHeight-gap)
}

func (m *articlesModel) ensureCursorVisible() {
	if len(m.rows) == 0 {
		m.cursor, m.offset = 0, 0
		return
	}
	m.cursor = min(max(0, m.cursor), len(m.rows)-1)
	m.offset = calculateScrollOffset(m.cursor, m.offset, max(1, m.listHeight()), len(m.rows))
}

func articlesSingleLine(text string) string {
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, ansi.Strip(text))
	return strings.Join(strings.Fields(text), " ")
}

func (m articlesModel) View() string {
	width, height := max(1, m.width), max(1, m.height)
	fit := func(text string) string { return ansi.Truncate(text, width, "…") }
	lines := []string{StyleTitle.MarginBottom(0).Render(fit(" Knowledge Base: " + articlesSingleLine(m.projectCode)))}
	if m.searchMode {
		m.searchInput.Width = max(1, width-2)
		lines = append(lines, fit(m.searchInput.View()))
	} else if query := strings.TrimSpace(m.searchInput.Value()); query != "" {
		lines = append(lines, StyleSubtext.Render(fit(fmt.Sprintf("%d matches · Filter: %s", len(m.rows), articlesSingleLine(query)))))
	} else {
		lines = append(lines, StyleSubtext.Render(fit(fmt.Sprintf("%d articles", len(m.rows)))))
	}
	bodyHeight := m.listHeight()
	var body []string
	switch {
	case m.loading:
		body = append(body, fit(m.spinner.View()+" Loading project articles..."))
	case m.err != nil:
		body = append(body, StyleErrorMessage.Render(fit("Error: "+articlesSingleLine(m.err.Error()))))
	case len(m.rows) == 0:
		message := "No articles visible in this project's Knowledge Base."
		if strings.TrimSpace(m.searchInput.Value()) != "" {
			message = "No matching articles. Press Esc to clear the filter."
		}
		body = append(body, StyleSubtext.Render(fit(message)))
	default:
		m.ensureCursorVisible()
		for i := m.offset; i < len(m.rows) && len(body) < bodyHeight; i++ {
			row := m.rows[i]
			id := row.article.IDReadable
			if id == "" {
				id = row.article.ID
			}
			title := articlesSingleLine(row.article.Summary)
			if title == "" {
				title = "(Untitled article)"
			}
			indent := strings.Repeat("  ", min(row.depth, width/2))
			if row.depth > 0 {
				indent += "└─ "
			}
			line := fit("  " + indent + articlesSingleLine(id) + "  " + title)
			if i == m.cursor {
				line = StyleSelected.Width(width).Render(line)
			} else {
				line = StyleNormal.Render(line)
			}
			body = append(body, line)
		}
	}
	body = body[:min(len(body), bodyHeight)]
	for len(body) < bodyHeight {
		body = append(body, "")
	}
	lines = append(lines, body...)
	if footer := m.footerLines(); len(footer) > 0 {
		lines = append(lines, "")
		for _, line := range footer {
			lines = append(lines, StyleHelp.Render(fit(line)))
		}
	}
	return strings.Join(lines[:min(len(lines), height)], "\n")
}
