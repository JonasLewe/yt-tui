package ui

import (
	"fmt"
	"os"
	"strings"
	"yt-tui/internal/config"
	"yt-tui/internal/ytcli"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/runeutil"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type articleFormField int

const (
	articleFieldSummary articleFormField = iota
	articleFieldContent
	articleFieldParent
)

type articleFormModel struct {
	client          *ytcli.Client
	projectCode     string
	articleID       string
	isEdit          bool
	ready           bool
	loading         bool
	saving          bool
	submitted       bool
	editorPending   bool
	err             error
	errPopupShow    bool
	requestID       uint64
	summaryInput    textinput.Model
	contentInput    textarea.Model
	contentSource   string
	contentBaseline string
	contentTail     string
	parentCandidate *ytcli.Article
	parentID        string
	parentSelected  bool
	focusIndex      articleFormField
	spinner         spinner.Model
	width           int
	height          int
}

type articleFormDataMsg struct {
	requestID uint64
	article   *ytcli.Article
	err       error
}

type articleFormSubmittedMsg struct {
	requestID uint64
	articleID string
	err       error
}

type articleSavedMsg struct {
	requestID   uint64
	articleID   string
	projectCode string
}

func newArticleFormModel(client *ytcli.Client, _ *config.Config) articleFormModel {
	summary := textinput.New()
	summary.Placeholder = "Summary of the article"
	content := textarea.New()
	content.Placeholder = "Article content (Markdown supported)..."
	content.ShowLineNumbers = false
	// Article bodies must not be truncated by the textarea's default line limit.
	content.MaxHeight = 0
	content.SetHeight(10)
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorViolet))
	return articleFormModel{client: client, summaryInput: summary, contentInput: content, spinner: s}
}

func (m *articleFormModel) setupForm(data string) tea.Cmd {
	m.requestID++
	m.projectCode, m.articleID, m.parentID = "", "", ""
	m.isEdit, m.loading, m.saving, m.submitted, m.editorPending = false, false, false, false, false
	m.ready = true
	m.err, m.errPopupShow = nil, false
	m.parentCandidate, m.parentSelected = nil, false
	m.summaryInput.SetValue("")
	m.setContent("")
	m.focusIndex = articleFieldSummary
	m.focusCurrent()
	m.applyInputSizes()
	if strings.HasPrefix(data, "edit:") {
		m.isEdit, m.ready = true, false
		m.articleID = strings.TrimPrefix(data, "edit:")
	} else if strings.HasPrefix(data, "new:") {
		parts := strings.SplitN(strings.TrimPrefix(data, "new:"), ":", 2)
		m.projectCode = parts[0]
		if len(parts) > 1 {
			m.parentID = parts[1]
		}
	} else {
		m.projectCode = data
	}
	loadID := m.articleID
	if !m.isEdit {
		loadID = m.parentID
	}
	if loadID == "" {
		if m.isEdit {
			m.err = fmt.Errorf("Article ID is required")
			m.errPopupShow = true
		}
		return nil
	}
	m.loading = true
	client, requestID := m.client, m.requestID
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		if client == nil {
			return articleFormDataMsg{requestID: requestID, err: fmt.Errorf("YouTrack client is not configured")}
		}
		article, err := client.GetArticle(loadID)
		return articleFormDataMsg{requestID: requestID, article: article, err: err}
	})
}

func (m *articleFormModel) focusCurrent() {
	m.summaryInput.Blur()
	m.contentInput.Blur()
	if m.focusIndex == articleFieldSummary {
		m.summaryInput.Focus()
	} else if m.focusIndex == articleFieldContent {
		m.contentInput.Focus()
	}
}

// Keep the original source independently of the textarea, which normalizes tabs
// and line endings and caps bulk input at 10,000 lines. A title-only save or an
// editor handoff must retain the complete source, including its byte formatting.
func (m *articleFormModel) setContent(content string) {
	m.contentSource = content
	m.contentInput.SetValue(content)
	m.contentBaseline = m.contentInput.Value()
	m.contentTail = ""
	normalized := string(runeutil.NewSanitizer().Sanitize([]rune(content)))
	if normalized == m.contentBaseline || !strings.HasPrefix(normalized, m.contentBaseline) {
		return
	}
	// The line cap stops before the separator following the last visible line.
	// CR and LF are each one separator in the textarea's sanitizer, even in CRLF.
	separators := strings.Count(m.contentBaseline, "\n")
	for index, r := range content {
		if r == '\r' || r == '\n' {
			if separators == 0 {
				m.contentTail = content[index:]
				return
			}
			separators--
		}
	}
}

func (m articleFormModel) contentValue() string {
	value := m.contentInput.Value()
	if value == m.contentBaseline {
		return m.contentSource
	}
	return value + m.contentTail
}

func (m *articleFormModel) moveFocus(delta int) {
	count := 2
	if !m.isEdit && m.parentCandidate != nil {
		count++
	}
	m.focusIndex = articleFormField((int(m.focusIndex) + delta + count) % count)
	m.focusCurrent()
}

func (m articleFormModel) formWidth() int {
	return max(1, min(80, m.width-6))
}

func (m articleFormModel) helpText() string {
	help := "[Tab/Shift-Tab] Navigate Fields  [Ctrl+s] Save & Submit  [Esc] Cancel/Back"
	if m.focusIndex == articleFieldContent {
		help = "[Tab/Shift-Tab] Navigate Fields  [Ctrl+g] External Editor  [Ctrl+s] Save & Submit  [Esc] Cancel/Back"
	} else if m.focusIndex == articleFieldParent {
		help = "[Tab/Shift-Tab] Navigate Fields  [←/→ or h/l] Select Parent  [Ctrl+s] Save & Submit  [Esc] Cancel/Back"
	}
	return wrapFooterHelp(help, max(1, m.width))
}

func (m *articleFormModel) applyInputSizes() {
	width := m.formWidth()
	m.summaryInput.Width = max(1, width-2)
	m.contentInput.SetWidth(max(1, width-2))
	reserved := 17 + lipgloss.Height(m.helpText())
	if m.parentCandidate != nil {
		reserved += 5
	}
	m.contentInput.SetHeight(max(1, min(10, m.height-reserved)))
}

func (m articleFormModel) Update(msg tea.Msg) (articleFormModel, tea.Cmd) {
	m.applyInputSizes()
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	case articleFormDataMsg:
		if msg.requestID != m.requestID || !m.loading || m.saving {
			return m, nil
		}
		m.loading = false
		m.err = msg.err
		if m.err == nil && msg.article == nil {
			m.err = fmt.Errorf("YouTrack did not return an article")
		}
		if m.err != nil {
			m.errPopupShow = true
			return m, nil
		}
		if m.isEdit {
			m.articleID = msg.article.ID
			m.summaryInput.SetValue(msg.article.Summary)
			m.setContent(msg.article.Content)
			if msg.article.Project != nil {
				m.projectCode = msg.article.Project.ShortName
			}
			m.parentCandidate = msg.article.ParentArticle
			m.ready = true
		} else {
			m.parentCandidate = msg.article
		}
		m.applyInputSizes()
	case articleFormSubmittedMsg:
		if msg.requestID != m.requestID || !m.saving {
			return m, nil
		}
		m.loading, m.saving = false, false
		m.err = msg.err
		if m.err != nil {
			m.errPopupShow = true
			return m, nil
		}
		m.submitted = true
		return m, func() tea.Msg {
			return articleSavedMsg{requestID: msg.requestID, articleID: msg.articleID, projectCode: m.projectCode}
		}
	case editorFinishedMsg:
		if !m.editorPending {
			return m, nil
		}
		m.editorPending = false
		if msg.tempPath != "" {
			defer os.Remove(msg.tempPath)
		}
		m.err = msg.err
		if m.err == nil && msg.tempPath != "" {
			var content []byte
			content, m.err = os.ReadFile(msg.tempPath)
			if m.err == nil {
				m.setContent(string(content))
			}
		}
		m.errPopupShow = m.err != nil
		return m, tea.ClearScreen
	case tea.KeyMsg:
		if m.errPopupShow {
			if msg.String() == "y" && m.err != nil {
				_ = clipboard.WriteAll(m.err.Error())
			}
			m.err, m.errPopupShow = nil, false
			return m, nil
		}
		if m.loading || m.editorPending || m.submitted {
			return m, nil
		}
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return popStateMsg{} }
		case "ctrl+g":
			if m.focusIndex == articleFieldContent {
				m.editorPending = true
				return m, openTextEditorCmd(m.contentValue(), "yt-tui-article-*.md", false)
			}
		case "ctrl+s":
			if !m.ready {
				m.err = fmt.Errorf("Article details could not be loaded; return to the article and try again")
			} else if strings.TrimSpace(m.summaryInput.Value()) == "" || (!m.isEdit && m.projectCode == "") {
				m.err = fmt.Errorf("Project and Summary fields are required")
			} else if m.client == nil {
				m.err = fmt.Errorf("YouTrack client is not configured")
			}
			if m.err != nil {
				m.errPopupShow = true
				return m, nil
			}
			m.loading, m.saving = true, true
			m.requestID++
			client, requestID := m.client, m.requestID
			id, project, summary, content := m.articleID, m.projectCode, m.summaryInput.Value(), m.contentValue()
			isEdit := m.isEdit
			parent := ""
			if m.parentSelected && m.parentCandidate != nil {
				parent = m.parentCandidate.ID
			}
			return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
				var err error
				if isEdit {
					err = client.UpdateArticle(id, summary, content)
				} else {
					id, err = client.CreateArticle(project, summary, content, parent)
				}
				return articleFormSubmittedMsg{requestID: requestID, articleID: id, err: err}
			})
		case "tab":
			m.moveFocus(1)
			return m, nil
		case "shift+tab":
			m.moveFocus(-1)
			return m, nil
		case "down", "up":
			if m.focusIndex != articleFieldContent {
				delta := 1
				if msg.String() == "up" {
					delta = -1
				}
				m.moveFocus(delta)
				return m, nil
			}
		case "left", "right", "h", "l":
			if m.focusIndex == articleFieldParent && m.parentCandidate != nil && !m.isEdit {
				m.parentSelected = !m.parentSelected
				return m, nil
			}
		}
		var cmd tea.Cmd
		switch m.focusIndex {
		case articleFieldSummary:
			m.summaryInput, cmd = m.summaryInput.Update(msg)
		case articleFieldContent:
			m.contentInput, cmd = m.contentInput.Update(msg)
		}
		return m, cmd
	}
	return m, nil
}

func (m articleFormModel) View() string {
	width, height := max(1, m.width), max(1, m.height)
	if m.loading || m.submitted {
		status := "Loading article details..."
		if m.saving || m.submitted {
			status = "Creating article..."
			if m.isEdit {
				status = "Updating article..."
			}
		}
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, m.spinner.View()+" "+status)
	}
	m.applyInputSizes()
	title := " Create New Article "
	if m.isEdit {
		title = " Edit Article: " + m.articleID + " "
	}
	targetWidth := m.formWidth()
	var sections []string
	sections = append(sections, "Project: "+articlesSingleLine(m.projectCode))
	sections = append(sections, "Summary:\n"+renderField(m.summaryInput.View(), m.focusIndex == articleFieldSummary, targetWidth))
	sections = append(sections, "Content:\n"+renderField(m.contentInput.View(), m.focusIndex == articleFieldContent, targetWidth))
	if m.parentCandidate != nil {
		label := m.parentCandidate.IDReadable
		if label == "" {
			label = m.parentCandidate.ID
		}
		label += " - " + articlesSingleLine(m.parentCandidate.Summary)
		if m.isEdit {
			label = ansi.Truncate(label, max(1, targetWidth-20), "…")
			sections = append(sections, "Parent: "+label+" (Read-only)")
		} else {
			if !m.parentSelected {
				label = "None"
			}
			label = ansi.Truncate(label, max(1, targetWidth-6), "…")
			sections = append(sections, "Parent:\n"+renderField("◀  "+label+"  ▶", m.focusIndex == articleFieldParent, targetWidth))
		}
	}
	form := lipgloss.NewStyle().Padding(1, 2).Render(strings.Join(sections, "\n\n"))
	view := lipgloss.JoinVertical(lipgloss.Left, StyleTitle.Render(title), form, "", StyleHelp.Render(m.helpText()))
	if m.errPopupShow && m.err != nil {
		popup := (formModel{err: m.err}).renderErrorPopup()
		x, y := max(0, (width-lipgloss.Width(popup))/2), max(0, (height-lipgloss.Height(popup))/2)
		view = overlayLines(view, popup, x, y)
	}
	return lipgloss.NewStyle().MaxWidth(width).MaxHeight(height).Render(view)
}
