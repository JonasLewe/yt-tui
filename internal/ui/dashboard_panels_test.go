package ui

import (
	"errors"
	"strings"
	"testing"
	"yt-tui/internal/config"
	"yt-tui/internal/ytcli"

	tea "github.com/charmbracelet/bubbletea"
)

func dashboardPanelFixture() dashboardModel {
	m := newDashboardModel(nil, &config.Config{})
	m.loadingIssues, m.loadingProj = false, false
	m.width, m.height = 120, 14
	m.issues = make([]ytcli.Issue, 20)
	m.projects = make([]ytcli.Project, 20)
	m.issueCursor, m.projectCursor = 7, 9
	m.issueScrollOffset, m.projectScrollOffset = 3, 5
	return m
}

func TestDashboardNumberedPanelsPreserveSelection(t *testing.T) {
	m := dashboardPanelFixture()
	for _, step := range []struct {
		key  tea.KeyMsg
		want activePanel
	}{
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")}, panelProjects},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")}, panelProjects},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("0")}, panelIssues},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("0")}, panelIssues},
		{tea.KeyMsg{Type: tea.KeyTab}, panelProjects},
		{tea.KeyMsg{Type: tea.KeyShiftTab}, panelIssues},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")}, panelIssues},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1"), Alt: true}, panelIssues},
	} {
		var cmd tea.Cmd
		m, cmd = m.Update(step.key)
		if m.active != step.want || cmd != nil {
			t.Fatalf("%s: active=%d command=%v, want panel %d without action", step.key.String(), m.active, cmd != nil, step.want)
		}
		if m.issueCursor != 7 || m.projectCursor != 9 || m.issueScrollOffset != 3 || m.projectScrollOffset != 5 {
			t.Fatalf("%s changed selection or scroll position", step.key.String())
		}
	}
	view := stripAnsi(m.View())
	for _, label := range []string{"[0] My Open Issues", "[1] Projects", "[0/1] Panel"} {
		if !strings.Contains(view, label) {
			t.Fatalf("dashboard is missing panel shortcut label %q", label)
		}
	}
}

func TestDashboardNumberedPanelsRespectOverlaysAndLoading(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*dashboardModel)
	}{
		{"loading issues", func(m *dashboardModel) { m.loadingIssues = true }},
		{"loading projects", func(m *dashboardModel) { m.loadingProj = true }},
		{"error", func(m *dashboardModel) { m.err = errors.New("load failed") }},
		{"action menu", func(m *dashboardModel) { m.actionMode = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			m := dashboardPanelFixture()
			test.setup(&m)
			m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
			if m.active != panelIssues || cmd != nil {
				t.Fatal("panel shortcut bypassed the active overlay or loading guard")
			}
		})
	}
}

func TestDashboardNumericActionShortcutRetainsPriority(t *testing.T) {
	m := dashboardPanelFixture()
	m.issues[m.issueCursor].IDReadable = "DEMO-1"
	m.cfg.Actions = config.ActionsMap{
		"default": {{Name: "Numeric action", Shortcut: "1"}},
	}
	m.actionMode = true
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	if cmd == nil || !m.loadingIssues || !m.loadingProj || m.active != panelIssues {
		t.Fatal("numeric action shortcut was consumed as panel navigation")
	}
	// Do not execute the returned action: this verifies dispatch without any API call.
}
