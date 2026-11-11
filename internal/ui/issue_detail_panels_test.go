package ui

import (
	"errors"
	"strings"
	"testing"
	"yt-tui/internal/config"
	"yt-tui/internal/ytcli"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func issuePanelFixture() detailModel {
	m := newDetailModel(nil, &config.Config{})
	m.width, m.height = 110, 40
	m, _ = m.Update(detailDataMsg{
		issue: &ytcli.Issue{
			ID: "1-1", IDReadable: "DEMO-1", Summary: "Panel navigation",
			Description: strings.Repeat("Description content\n", 80),
			Links: []ytcli.IssueLink{{
				ID: "link-1", LinkType: &ytcli.IssueLinkType{Name: "relates to"},
				Issues: []ytcli.Issue{{IDReadable: "DEMO-2"}, {IDReadable: "DEMO-3"}},
			}},
			Attachments: []ytcli.Attachment{{ID: "a-1", Name: "first.txt"}, {ID: "a-2", Name: "second.txt"}},
		},
		activities: []ytcli.ActivityItem{
			{Type: "CommentActivityItem", Added: map[string]interface{}{"text": "First comment"}},
			{Type: "CommentActivityItem", Added: map[string]interface{}{"text": "Second comment"}},
		},
	})
	m.commentsCursor, m.linksCursor, m.attachmentsCursor = 1, 1, 1
	m.descViewport.SetYOffset(5)
	return m
}

func TestIssueNumberedPanelsPreserveSelection(t *testing.T) {
	for _, fullView := range []bool{false, true} {
		name := "split"
		if fullView {
			name = "full"
		}
		t.Run(name, func(t *testing.T) {
			m := issuePanelFixture()
			m.fullView = fullView
			labels := []string{"[0] Description", "[1] Comments", "[2] Links", "[3] Attachments"}
			for _, key := range []rune("33110022") {
				want := int(key - '0')
				var cmd tea.Cmd
				m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
				if m.activeViewport != want || cmd != nil || m.fullView != fullView {
					t.Fatalf("%c: active=%d, command=%v, fullView=%v", key, m.activeViewport, cmd != nil, m.fullView)
				}
				if m.commentsCursor != 1 || m.linksCursor != 1 || m.attachmentsCursor != 1 || m.descViewport.YOffset != 5 {
					t.Fatalf("%c changed a selection or description scroll position", key)
				}
				view := stripAnsi(m.View())
				for index, label := range labels {
					if got, wantVisible := strings.Contains(view, label), !fullView || index == want; got != wantVisible {
						t.Fatalf("%c: label %q visible=%v, want %v", key, label, got, wantVisible)
					}
				}
			}
			m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
			if m.activeViewport != 3 {
				t.Fatal("Tab did not continue from the directly selected panel")
			}
			m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
			if m.activeViewport != 2 {
				t.Fatal("Shift+Tab did not retain its previous-panel behavior")
			}
			for _, key := range []tea.KeyMsg{
				{Type: tea.KeyRunes, Runes: []rune("9")},
				{Type: tea.KeyRunes, Runes: []rune("1"), Alt: true},
			} {
				m, _ = m.Update(key)
				if m.activeViewport != 2 {
					t.Fatalf("unassigned key %s changed panels", key.String())
				}
			}
		})
	}
}

func TestIssuePanelDigitsRemainTextInInputs(t *testing.T) {
	for _, mode := range []detailMode{modeCommentInput, modeCommentEdit, modeEditEstimation, modeAssignInput} {
		m := issuePanelFixture()
		m.mode, m.activeViewport = mode, 2
		switch mode {
		case modeCommentInput, modeCommentEdit:
			m.commentInput.Focus()
		case modeEditEstimation:
			m.estimationInput.Focus()
		case modeAssignInput:
			m.textInput.Focus()
		}
		for _, key := range "0123" {
			m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		}
		var got string
		switch mode {
		case modeCommentInput, modeCommentEdit:
			got = m.commentInput.Value()
		case modeEditEstimation:
			got = m.estimationInput.Value()
		case modeAssignInput:
			got = m.textInput.Value()
		}
		if got != "0123" || m.mode != mode || m.activeViewport != 2 {
			t.Fatalf("mode %d: digits changed focus or failed to type: value=%q mode=%d panel=%d", mode, got, m.mode, m.activeViewport)
		}
	}
}

func TestIssueNumberedPanelsRespectModalGuards(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*detailModel)
	}{
		{"loading", func(m *detailModel) { m.loading = true }},
		{"error popup", func(m *detailModel) { m.err, m.errPopupShow = errors.New("failed"), true }},
		{"state selection", func(m *detailModel) { m.mode = modeStateSelect }},
		{"action menu", func(m *detailModel) { m.mode = modeActionSelect }},
		{"file picker", func(m *detailModel) { m.filepickerActive = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			m := issuePanelFixture()
			test.setup(&m)
			m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
			if m.activeViewport != 0 {
				t.Fatal("panel shortcut bypassed the active modal or loading guard")
			}
		})
	}
}

func TestIssueNumericActionShortcutRetainsPriority(t *testing.T) {
	m := issuePanelFixture()
	m.cfg.Actions = config.ActionsMap{"default": {{Name: "Numeric action", Shortcut: "1"}}}
	m.mode = modeActionSelect
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	if cmd == nil || !m.loading || m.activeViewport != 0 {
		t.Fatal("numeric action shortcut was consumed as panel navigation")
	}
	// The action command is deliberately not executed; no API call is needed.
}

func TestNarrowIssuePanelTitleKeepsShortcut(t *testing.T) {
	for _, width := range []int{7, 16} {
		style := StyleNormalBorder.Width(width).Height(2)
		view := renderBoxWithTitle(style, "[3] Attachments", "content", false)
		top := strings.Split(view, "\n")[0]
		if !strings.Contains(stripAnsi(top), "[3]") {
			t.Fatalf("width %d: narrow title hid the panel shortcut", width)
		}
		if got, want := lipgloss.Width(top), lipgloss.Width(style.Render("content")); got != want {
			t.Fatalf("title widened the panel border from %d to %d", want, got)
		}
	}
}
