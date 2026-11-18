package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"yt-tui/internal/config"
	"yt-tui/internal/filepicker"
	"yt-tui/internal/ytcli"

	tea "github.com/charmbracelet/bubbletea"
)

func storedUserConfig(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".config", "yt-tui", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]json.RawMessage
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	return stored
}

func assertInheritedSettingsNotStored(t *testing.T, stored map[string]json.RawMessage) {
	t.Helper()
	for _, key := range []string{"fields", "page_size", "max_issues", "custom_types", "custom_states", "custom_priorities", "work_types", "theme", "browser_command"} {
		if _, exists := stored[key]; exists {
			t.Errorf("saving a user action froze inherited setting %q", key)
		}
	}
}

func TestSavingIssueFiltersKeepsFutureColumnDefaultsLive(t *testing.T) {
	m := typeFilterModel(t)
	m.openFilters()
	m.filterPanel.sections[2].Selected = selection([]string{"Epic"})
	if cmd := m.saveFilters(); cmd == nil || m.filterError != "" {
		t.Fatalf("filter save failed: %s", m.filterError)
	}
	stored := storedUserConfig(t)
	assertInheritedSettingsNotStored(t, stored)
	for _, key := range []string{"filtered_states", "filtered_priorities"} {
		if _, frozen := stored[key]; frozen {
			t.Errorf("type filter save froze inherited %s", key)
		}
	}
	previous := config.DefaultFields
	previousStates := config.DefaultStates
	config.DefaultFields = append(slices.Clone(previous), "Updated")
	config.DefaultStates = append(slices.Clone(previousStates), "Reviewed")
	t.Cleanup(func() {
		config.DefaultFields = previous
		config.DefaultStates = previousStates
	})
	reloaded, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(reloaded.Fields, "Updated") || !slices.Equal(reloaded.GetFilteredTypes("DEMO"), []string{"Epic"}) {
		t.Fatal("saved filter prevented new defaults or lost its own selection")
	}
	if !slices.Contains(reloaded.GetFilteredStates("DEMO"), "Reviewed") {
		t.Fatal("changing only Type prevented inheritance of a new state")
	}
}

func TestUnchangedIssueFiltersDoNotCreateOverrides(t *testing.T) {
	m := typeFilterModel(t)
	m.openFilters()
	m.saveFilters()
	if m.filterError != "" {
		t.Fatal(m.filterError)
	}
	if stored := storedUserConfig(t); len(stored) != 1 || stored["config_version"] == nil {
		t.Fatalf("unchanged filter dialog created overrides: %v", stored)
	}

	// An existing explicit selection equal to today's default must survive.
	m.cfg.SetFilteredStates("DEMO", m.cfg.GetFilteredStates("DEMO"))
	if err := config.SaveConfig(m.cfg); err != nil {
		t.Fatal(err)
	}
	m.openFilters()
	m.saveFilters()
	var states map[string][]string
	if err := json.Unmarshal(storedUserConfig(t)["filtered_states"], &states); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(states["DEMO"], config.DefaultStates) {
		t.Fatal("unchanged dialog discarded an existing explicit selection")
	}
}

func TestAllTypesOverridesAnInheritedRestriction(t *testing.T) {
	m := typeFilterModel(t)
	m.cfg.SetFilteredTypes("default", []string{"Epic"})
	if err := config.SaveConfig(m.cfg); err != nil {
		t.Fatal(err)
	}
	m.selectedTypes = []string{"Epic"}
	m.openFilters()
	clear(m.filterPanel.sections[2].Selected)
	m.saveFilters()
	if m.filterError != "" {
		t.Fatal(m.filterError)
	}
	var types map[string][]string
	if err := json.Unmarshal(storedUserConfig(t)["filtered_types"], &types); err != nil {
		t.Fatal(err)
	}
	if project, exists := types["DEMO"]; !exists || project == nil || len(project) != 0 {
		t.Fatal("All types did not persist an explicit empty project selection")
	}
	if !slices.Equal(types["default"], []string{"Epic"}) {
		t.Fatal("project All types changed the inherited default filter")
	}
	reloaded, err := config.LoadConfig()
	if err != nil || len(reloaded.GetFilteredTypes("DEMO")) != 0 {
		t.Fatalf("All types did not survive reload: %v", err)
	}
}

func TestSavingIssueFiltersPreservesExplicitColumns(t *testing.T) {
	m := typeFilterModel(t)
	// An explicit current-default layout is still a personal override. Future
	// updates must not treat it like an inherited layout just because it matches.
	m.cfg.Fields = slices.Clone(config.DefaultFields)
	if err := config.SaveConfig(m.cfg, "fields"); err != nil {
		t.Fatal(err)
	}
	want := slices.Clone(m.cfg.Fields)
	m.openFilters()
	m.filterPanel.sections[2].Selected = selection([]string{"Story"})
	m.saveFilters()
	var storedFields []string
	if err := json.Unmarshal(storedUserConfig(t)["fields"], &storedFields); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(storedFields, want) {
		t.Fatal("filter save changed an explicit layout")
	}
	previous := config.DefaultFields
	config.DefaultFields = append(slices.Clone(previous), "Updated")
	t.Cleanup(func() { config.DefaultFields = previous })
	reloaded, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(reloaded.Fields, want) {
		t.Fatal("future defaults replaced an explicit layout")
	}
}

func TestFailedFilterSaveDoesNotLeakDraftOverridesIntoNextSave(t *testing.T) {
	m := typeFilterModel(t)
	m.cfg.SetFilteredTypes("DEMO", []string{"Epic"})
	if err := config.SaveConfig(m.cfg); err != nil {
		t.Fatal(err)
	}
	m.selectedTypes = []string{"Epic"}
	path := filepath.Join(os.Getenv("HOME"), ".config", "yt-tui", "config.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	m.openFilters()
	m.filterPanel.sections[2].Selected = selection([]string{"Story"})
	clear(m.filterPanel.sections[0].Selected)
	m.saveFilters()
	if m.filterError == "" || !slices.Equal(m.cfg.GetFilteredTypes("DEMO"), []string{"Epic"}) {
		t.Fatal("failed save applied the pending selection")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, before, 0600); err != nil {
		t.Fatal(err)
	}
	m.cfg.RenderMarkdown = false
	if err := config.SaveConfig(m.cfg, "render_markdown"); err != nil {
		t.Fatal(err)
	}
	reloaded, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(reloaded.GetFilteredTypes("DEMO"), []string{"Epic"}) || reloaded.RenderMarkdown {
		t.Fatal("later save leaked the failed draft or lost its own change")
	}
	if _, leaked := storedUserConfig(t)["filtered_states"]; leaked {
		t.Fatal("failed draft left an explicit state filter behind")
	}
	assertInheritedSettingsNotStored(t, storedUserConfig(t))
}

func loadedDetailForOverrides(t *testing.T) detailModel {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	m := newDetailModel(nil, cfg)
	m.width, m.height = 110, 40
	m, _ = m.Update(detailDataMsg{issue: &ytcli.Issue{ID: "1-1", IDReadable: "DEMO-1", Summary: "Override test"}})
	return m
}

func TestMarkdownTogglePersistsExplicitBooleanWithoutFreezingDefaults(t *testing.T) {
	m := loadedDetailForOverrides(t)
	for _, want := range []bool{false, true} {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
		stored := storedUserConfig(t)
		assertInheritedSettingsNotStored(t, stored)
		value, exists := stored["render_markdown"]
		if !exists {
			t.Fatal("explicit markdown toggle was omitted when equal to the default")
		}
		var got bool
		if err := json.Unmarshal(value, &got); err != nil || got != want {
			t.Fatalf("markdown override = %s, expected %t", value, want)
		}
	}
}

func TestEmptyActivitySelectionSurvivesReload(t *testing.T) {
	m := loadedDetailForOverrides(t)
	m.mode = modeFilterSelect
	m.tempFilters = map[string]bool{}
	m.tempActivitySortOrder = "asc"
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("activity filter did not request its normal refresh")
	}
	stored := storedUserConfig(t)
	assertInheritedSettingsNotStored(t, stored)
	if string(stored["activity_filters"]) != "[]" {
		t.Fatalf("explicit no-activity selection = %s, expected []", stored["activity_filters"])
	}
	if _, frozen := stored["activity_sort_order"]; frozen {
		t.Fatal("changing activity filters froze the inherited sort order")
	}
	reloaded, err := config.LoadConfig()
	if err != nil || len(reloaded.ActivityFilters) != 0 {
		t.Fatalf("empty activity override did not survive reload: %v", err)
	}
}

func TestActivityDialogOnlyPersistsChangedPreferences(t *testing.T) {
	m := loadedDetailForOverrides(t)
	for _, order := range []string{"asc", "desc"} {
		m.loading = false
		m.mode = modeFilterSelect
		m.tempFilters = selection(m.cfg.ActivityFilters)
		m.tempActivitySortOrder = order
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		stored := storedUserConfig(t)
		if _, frozen := stored["activity_filters"]; frozen {
			t.Fatal("unchanged activity filters became an override")
		}
		if order == "asc" {
			if _, frozen := stored["activity_sort_order"]; frozen {
				t.Fatal("unchanged sort order became an override")
			}
		} else if string(stored["activity_sort_order"]) != `"desc"` {
			t.Fatal("changed activity sort order was not saved")
		}
	}
}

func TestFilepickerCloseStoresOnlyItsPreferences(t *testing.T) {
	m := loadedDetailForOverrides(t)
	m.filepickerActive = true
	m.filepicker.CurrentDirectory = os.Getenv("HOME")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	stored := storedUserConfig(t)
	assertInheritedSettingsNotStored(t, stored)
	if _, exists := stored["filepicker_last_dir"]; !exists {
		t.Fatal("filepicker directory was not saved")
	}
	for _, key := range []string{"filepicker_sort_by", "filepicker_sort_order"} {
		if _, frozen := stored[key]; frozen {
			t.Errorf("unchanged filepicker preference %q became an override", key)
		}
	}
	m.filepickerActive = true
	m.filepicker.SortOrder = filepicker.SortDescending
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	stored = storedUserConfig(t)
	if string(stored["filepicker_sort_order"]) != `"desc"` {
		t.Fatal("changed filepicker order was not saved")
	}
	if _, frozen := stored["filepicker_sort_by"]; frozen {
		t.Fatal("changing filepicker order froze the inherited sort field")
	}
}

func TestChangingSortColumnPreservesDirectionInheritance(t *testing.T) {
	for _, direction := range []string{"", "asc", "desc"} {
		t.Run("direction="+direction, func(t *testing.T) {
			m := typeFilterModel(t)
			if direction != "" {
				m.cfg.SortDirection = direction
				if err := config.SaveConfig(m.cfg, "sort_direction"); err != nil {
					t.Fatal(err)
				}
			}
			m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
			for index, field := range m.fields {
				if field.title == "Summary" {
					m.sortColCursor = index
					break
				}
			}
			m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			stored := storedUserConfig(t)
			if string(stored["sort_column"]) != `"Summary"` {
				t.Fatal("selected sort column was not saved")
			}
			if direction == "" {
				if _, exists := stored["sort_direction"]; exists {
					t.Fatal("changing the column froze inherited ascending order")
				}
			} else if string(stored["sort_direction"]) != `"`+direction+`"` {
				t.Fatal("changing the column lost the existing explicit direction")
			}
		})
	}
}
