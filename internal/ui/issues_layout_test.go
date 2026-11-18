package ui

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"yt-tui/internal/config"
	"yt-tui/internal/ytcli"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func issuesLayoutApp(fields []string) AppModel {
	cfg := &config.Config{PageSize: 20, MaxIssues: 500, Fields: fields}
	m := newIssuesModel(nil, cfg)
	m.projectCode, m.loading, m.loadedAll = "DEMO", false, true
	m.typeInfo = ytcli.IssueTypeInfo{Found: true, FieldName: "Type", Values: []string{"Epic", "Story"}}
	for i := 0; i < 20; i++ {
		kind := "Epic"
		if i%2 == 1 {
			kind = "Story"
		}
		m.issues = append(m.issues, ytcli.Issue{
			ID: fmt.Sprint(i), IDReadable: fmt.Sprintf("DEMO-%d", i), Summary: "Login with passkey",
			Project:      &ytcli.Project{ID: "0-1", ShortName: "DEMO"},
			CustomFields: []ytcli.CustomField{{Name: "Type", Value: map[string]interface{}{"name": kind}}},
		})
	}
	m.updateTableRows()
	return AppModel{state: stateIssues, cfg: cfg, issues: m,
		history: []navEntry{{state: stateDashboard}, {state: stateProjects}}}
}

func resizeIssuesApp(app AppModel, width, height int) AppModel {
	updated, _ := app.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return updated.(AppModel)
}

func assertIssuesViewFits(t *testing.T, view string, width, height int) {
	t.Helper()
	if lipgloss.Width(view) > width || lipgloss.Height(view) > height {
		t.Fatalf("view is %dx%d, exceeds terminal %dx%d:\n%s", lipgloss.Width(view), lipgloss.Height(view), width, height, ansi.Strip(view))
	}
}

func TestAppIssueFilterFitsRootPaddingAndKeepsHelp(t *testing.T) {
	for _, size := range [][2]int{{36, 18}, {80, 24}, {110, 32}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			app := issuesLayoutApp(nil)
			app = resizeIssuesApp(app, size[0], size[1])
			app.issues.openFilters()
			app.issues.filterPanel.section = 2
			view := app.View()
			assertIssuesViewFits(t, view, size[0], size[1])
			if app.issues.width != size[0]-4 {
				t.Fatal("issue model did not reserve the root's horizontal padding")
			}
			plain := ansi.Strip(view)
			for _, text := range []string{"Types", "All types", "Enter Save", "Esc Cancel"} {
				if !strings.Contains(plain, text) {
					t.Fatalf("filter view lost %q at %v:\n%s", text, size, plain)
				}
			}
		})
	}
}

func TestAppIssuesTableResponsiveTypeAndSummary(t *testing.T) {
	for _, width := range []int{80, 110, 160} {
		app := resizeIssuesApp(issuesLayoutApp(nil), width, 24)
		view := app.View()
		assertIssuesViewFits(t, view, width, 24)
		plain := ansi.Strip(view)
		for _, text := range []string{"Type", "Summary", "Epic", "Story", "Login with passkey", "[F] Filter", "[Esc] Back"} {
			if !strings.Contains(plain, text) {
				t.Fatalf("table lost %q at width %d:\n%s", text, width, plain)
			}
		}
		columns := app.issues.table.Columns()
		if columns[1].Width < 5 || columns[2].Width < 20 {
			t.Fatalf("type/summary widths too small: %+v", columns)
		}
		if lipgloss.Width(app.issues.table.View()) > app.issues.width {
			t.Fatal("table depends on final clipping instead of fitting its viewport")
		}
	}
}

func TestIssueColumnsPreserveCustomOrderAcrossResize(t *testing.T) {
	configured := []string{"Summary", "Type", "ID", "Component"}
	app := issuesLayoutApp(configured)
	for _, size := range [][2]int{{80, 24}, {36, 12}, {170, 30}} {
		app = resizeIssuesApp(app, size[0], size[1])
		var got []string
		for _, column := range app.issues.table.Columns() {
			got = append(got, column.Title)
		}
		if !slices.Equal(got, configured) || !slices.Equal(app.cfg.Fields, configured) {
			t.Fatalf("resize reordered or changed configured columns: %v", got)
		}
		assertIssuesViewFits(t, app.View(), size[0], size[1])
	}
	if app.issues.table.Columns()[0].Width < 55 {
		t.Fatal("summary did not expand again after returning to a wide terminal")
	}
}

func TestAppIssueTypeFilterScrollsLongOptionsInSmallWindow(t *testing.T) {
	app := resizeIssuesApp(issuesLayoutApp(nil), 40, 18)
	for i := 0; i < 40; i++ {
		app.issues.typeInfo.Values = append(app.issues.typeInfo.Values, fmt.Sprintf("Custom type %02d", i))
	}
	app.issues.openFilters()
	app.issues.filterPanel.section = 2
	app.issues.filterPanel.move("up")
	view := app.View()
	assertIssuesViewFits(t, view, 40, 18)
	if !strings.Contains(ansi.Strip(view), "Story") || !strings.Contains(ansi.Strip(view), "Esc Cancel") {
		t.Fatalf("last option or help is outside viewport:\n%s", ansi.Strip(view))
	}
}

func TestGlobalIssueActionInvalidatesProjectTypeCaches(t *testing.T) {
	m := issuesLayoutApp(nil).issues
	m.projectCode, m.query = "", "reporter: me"
	for _, key := range []issuesCacheKey{
		{projectCode: "DEMO", query: "Type: {Epic}"},
		{projectCode: "SECOND", query: "Type: {Story}"},
		{query: "reporter: me"},
	} {
		m.cache[key] = projectCache{loadedAll: true}
	}
	m, _ = m.Update(actionFinishedMsg{})
	if len(m.cache) != 0 {
		t.Fatal("global action left project-specific filtered cache entries")
	}
}

func TestIssueMutationInvalidatesBothProjectAliases(t *testing.T) {
	m := issuesLayoutApp(nil).issues
	m.typeInfoCache[issueTypesKey{projectCode: "DEMO"}] = ytcli.IssueTypeInfo{
		Project: ytcli.Project{ID: "0-1", ShortName: "DEMO"},
	}
	for _, project := range []string{"DEMO", "0-1", "UNRELATED"} {
		m.cache[issuesCacheKey{projectCode: project, query: "Type: {Epic}"}] = projectCache{}
	}
	m.invalidateCache("0-1")
	if len(m.cache) != 1 {
		t.Fatalf("expected only unrelated project cache to survive, got %v", m.cache)
	}
	if _, ok := m.cache[issuesCacheKey{projectCode: "UNRELATED", query: "Type: {Epic}"}]; !ok {
		t.Fatal("unrelated project cache removed")
	}
}
