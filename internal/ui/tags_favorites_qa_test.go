package ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"yt-tui/internal/config"
	"yt-tui/internal/ytcli"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
)

func TestDisabledFavoritesPreserveSavedDataAndNavigation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	const serverURL = "https://favorites.example.invalid"
	cfg.FavoriteView = "LEGACY"
	cfg.FavoriteViews = map[string]string{serverURL: "DEMO"}
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(os.Getenv("HOME"), ".config", "yt-tui", "config.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	client := ytcli.NewClient()
	client.SetCredentials(serverURL, "test-token")
	for _, state := range []State{stateDashboard, stateIssues} {
		t.Run(string(state), func(t *testing.T) {
			app := AppModel{state: state, cfg: cfg, client: client}
			app.dashboard = newDashboardModel(client, cfg)
			app.issues = newIssuesModel(client, cfg)
			app.issues.projectCode = "DEMO"
			app.issues.width, app.issues.height, app.issues.loading = 100, 24, false
			updated, cmd := app.Update(quitKey("f"))
			app = updated.(AppModel)
			if cmd != nil || app.state != state || len(app.history) != 0 {
				t.Fatal("disabled favorite key changed navigation or started an action")
			}
			if cfg.FavoriteView != "LEGACY" || cfg.FavoriteViews[serverURL] != "DEMO" {
				t.Fatal("disabled favorite key changed retained bookmark data")
			}
			view := app.issues.View()
			if strings.Contains(view, "★") || strings.Contains(view, "Favorite") {
				t.Fatal("disabled favorite UI remains in issue header/footer")
			}
		})
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("disabled favorite key rewrote saved config: %v", err)
	}
}

func TestDisabledFavoritesLeaveCustomFActionAvailable(t *testing.T) {
	cfg := &config.Config{Actions: config.ActionsMap{"default": {{Name: "Custom F", Shortcut: "f"}}}}
	for _, state := range []State{stateDashboard, stateIssues} {
		t.Run(string(state), func(t *testing.T) {
			issue := ytcli.Issue{IDReadable: "DEMO-1"}
			app := AppModel{state: state, cfg: cfg}
			app.dashboard = newDashboardModel(nil, cfg)
			app.dashboard.actionMode = true
			app.dashboard.issues = []ytcli.Issue{issue}
			app.issues = newIssuesModel(nil, cfg)
			app.issues.actionMode = true
			app.issues.visibleIssueIDs = []string{issue.IDReadable}
			app.issues.table.SetRows([]table.Row{{issue.IDReadable}})
			_, cmd := app.Update(quitKey("f"))
			if cmd == nil {
				t.Fatal("custom f action was not dispatched")
			}
			// Empty command lists perform no requests; the result identifies the
			// action handler that received the key.
			switch msg := cmd().(type) {
			case dashboardActionFinishedMsg, actionFinishedMsg:
			default:
				t.Fatalf("custom f action produced %T", msg)
			}
		})
	}
}

func TestTagSearchOwnsGlobalKeysAndKeepsDraftUntilCancel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	app := AppModel{state: stateIssues, cfg: cfg, issues: newIssuesModel(nil, cfg)}
	app.issues.loading, app.issues.filterMode = false, true
	app.issues.filterPanel = newIssuesFilterPanel([]issueFilterSection{{
		Title: "Tags", Options: []string{"backend", "qQSf?jk"},
		Selected: map[string]bool{"backend": true}, AllowAll: true,
		AllLabel: "No restriction", Searchable: true,
	}})
	updated, _ := app.Update(quitKey("/"))
	app = updated.(AppModel)
	for _, key := range []string{"q", "Q", "S", "f", "?", "j", "k"} {
		updated, _ := app.Update(quitKey(key))
		app = updated.(AppModel)
	}
	if !app.issues.filterPanel.searchMode || app.issues.filterPanel.searchInput.Value() != "qQSf?jk" || app.searchShow || app.helpShow {
		t.Fatalf("global/navigation keys escaped the tag search input: searchMode=%v input=%q globalSearch=%v help=%v", app.issues.filterPanel.searchMode, app.issues.filterPanel.searchInput.Value(), app.searchShow, app.helpShow)
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(AppModel)
	if app.issues.filterPanel.searchMode || !app.issues.filterMode {
		t.Fatal("Enter in tag search saved/closed the filter panel")
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyDown})
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeySpace})
	app = updated.(AppModel)
	if selected := app.issues.filterPanel.sections[0].Selected; !selected["backend"] || !selected["qQSf?jk"] {
		t.Fatal("search lost hidden draft selections or toggled the wrong tag")
	}
	updated, _ = app.Update(quitKey("/"))
	app = updated.(AppModel)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(AppModel)
	if app.issues.filterPanel.searchMode || !app.issues.filterMode || app.issues.filterPanel.sections[0].Search != "" {
		t.Fatal("Escape did not clear and close the tag search independently")
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(AppModel)
	if app.issues.filterMode || len(cfg.FilteredTags) != 0 {
		t.Fatal("Escape did not discard the filter draft")
	}
}

func TestTypeOnlySaveDoesNotWaitForOrPersistUnusedTagFilters(t *testing.T) {
	for _, lookup := range []string{"pending", "failed"} {
		t.Run(lookup, func(t *testing.T) {
			m := typeFilterModel(t)
			m.openFilters()
			m.tagScopeReady = false
			m.tagScopeLoading = lookup == "pending"
			if lookup == "failed" {
				m.tagScopeErr = errors.New("account lookup failed")
				m.tagsErr = errors.New("tag catalog unavailable")
			}
			m.filterPanel.sections[2].Selected = selection([]string{"Story"})
			m.saveFilters()
			if m.filterMode || m.filterError != "" {
				t.Fatalf("unused tags blocked type save: %s", m.filterError)
			}
			path := filepath.Join(os.Getenv("HOME"), ".config", "yt-tui", "config.json")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var saved map[string]json.RawMessage
			if err := json.Unmarshal(raw, &saved); err != nil {
				t.Fatal(err)
			}
			if _, present := saved["filtered_tags"]; present {
				t.Fatal("saving Type pinned unchanged unrestricted Tags")
			}
			if values := m.cfg.GetFilteredTypes("DEMO"); len(values) != 1 || values[0] != "Story" {
				t.Fatalf("Type selection did not save: %v", values)
			}
		})
	}
}

func TestFailedTagAccountLookupCannotBroadenSavedRestriction(t *testing.T) {
	m := typeFilterModel(t)
	m.cfg.SetFilteredTags("https://account.example.invalid#1-7", "DEMO", []string{"backend"})
	m.tagScopeReady, m.tagScopeLoading = false, false
	m.tagScopeErr = errors.New("account lookup failed")
	if cmd := m.restoreOrLoadIssues(); cmd != nil || m.err == nil || m.loading {
		t.Fatal("failed account lookup allowed an unrestricted issue request")
	}
	if values := m.cfg.GetFilteredTags("https://account.example.invalid#1-7", "DEMO"); len(values) != 1 || values[0] != "backend" {
		t.Fatal("failed account lookup removed the saved restriction")
	}
}
