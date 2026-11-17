package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"yt-tui/internal/config"
)

func workflowConfigPath(t *testing.T, contents string) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(os.Getenv("HOME"), ".config", "yt-tui", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func readWorkflowJSON(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestSavedFilterDoesNotPreventNextDefaultLayout(t *testing.T) {
	path := workflowConfigPath(t, `{"config_version":1,"token":"$YOUTRACK_TOKEN"}`)
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.SetFilteredTypes("DEMO", []string{"Epic", "Story"})
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if _, exists := readWorkflowJSON(t, path)["fields"]; exists {
		t.Fatal("saving a filter persisted the inherited column layout")
	}

	// Simulate a later release adding a default column. The previous save must
	// leave the user eligible for the new defaults without further migration.
	previous := config.DefaultFields
	config.DefaultFields = append(append([]string(nil), previous...), "Updated")
	t.Cleanup(func() { config.DefaultFields = previous })
	reloaded, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(reloaded.Fields, config.DefaultFields) {
		t.Fatalf("updated defaults not inherited: %v", reloaded.Fields)
	}
	if !slices.Equal(reloaded.GetFilteredTypes("DEMO"), []string{"Epic", "Story"}) {
		t.Fatal("inheriting new defaults lost the personal type filter")
	}
	reloaded.SortColumn = "Assignee"
	if err := config.SaveConfig(reloaded); err != nil {
		t.Fatal(err)
	}
	raw := readWorkflowJSON(t, path)
	if _, exists := raw["fields"]; exists {
		t.Fatal("a later preference save froze the newly inherited columns")
	}
	if string(raw["token"]) != `"$YOUTRACK_TOKEN"` {
		t.Fatal("credential reference changed")
	}
}

func TestExplicitDefaultLayoutAndEmptyValuesRemainOverrides(t *testing.T) {
	wantFields := append([]string(nil), config.DefaultFields...)
	input := map[string]interface{}{
		"config_version":    1,
		"fields":            wantFields,
		"render_markdown":   false,
		"max_issues":        0,
		"filtered_types":    map[string]interface{}{"DEMO": []string{}},
		"extension_setting": map[string]interface{}{"enabled": true, "values": []string{"custom"}},
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	path := workflowConfigPath(t, string(data))
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RenderMarkdown || cfg.MaxIssues != 0 {
		t.Fatal("explicit false/zero replaced by default")
	}
	cfg.SortColumn = "ID"
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	raw := readWorkflowJSON(t, path)
	if _, exists := raw["fields"]; !exists {
		t.Fatal("explicit layout identical to defaults must remain pinned")
	}
	if string(raw["render_markdown"]) != "false" || string(raw["max_issues"]) != "0" {
		t.Fatal("explicit zero values were omitted")
	}
	var savedTypes map[string][]string
	if err := json.Unmarshal(raw["filtered_types"], &savedTypes); err != nil {
		t.Fatal(err)
	}
	if values, exists := savedTypes["DEMO"]; !exists || values == nil || len(values) != 0 {
		t.Fatal("explicit all-types override lost its empty list")
	}
	var extra interface{}
	if err := json.Unmarshal(raw["extension_setting"], &extra); err != nil {
		t.Fatal(err)
	}
	var expected interface{}
	extraJSON, _ := json.Marshal(input["extension_setting"])
	_ = json.Unmarshal(extraJSON, &expected)
	if !reflect.DeepEqual(extra, expected) {
		t.Fatal("unknown extension setting was discarded or changed")
	}

	previous := config.DefaultFields
	config.DefaultFields = append(append([]string(nil), previous...), "Updated")
	t.Cleanup(func() { config.DefaultFields = previous })
	reloaded, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(reloaded.Fields, wantFields) {
		t.Fatal("a new default overrode an explicit personal layout")
	}
}

func TestRemovingPersonalLayoutRestoresCurrentDefaults(t *testing.T) {
	path := workflowConfigPath(t, `{"config_version":1,"fields":["ID","Summary"],"theme":"teams"}`)
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cfg.Fields, []string{"ID", "Summary"}) {
		t.Fatal("explicit column layout not applied")
	}
	// This mirrors editing config.json between application sessions.
	if err := os.WriteFile(path, []byte(`{"config_version":1,"theme":"teams"}`), 0600); err != nil {
		t.Fatal(err)
	}
	reloaded, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(reloaded.Fields, config.DefaultFields) || reloaded.Theme != "teams" {
		t.Fatal("removing one override did not restore defaults independently")
	}
	reloaded.SetFilteredTypes("DEMO", []string{"Story"})
	if err := config.SaveConfig(reloaded); err != nil {
		t.Fatal(err)
	}
	if _, exists := readWorkflowJSON(t, path)["fields"]; exists {
		t.Fatal("saving later resurrected the deleted layout")
	}
}

func TestUnrelatedSaveKeepsEditsMadeWhileApplicationIsRunning(t *testing.T) {
	path := workflowConfigPath(t, `{"config_version":1,"fields":["ID","Summary"],"extension_setting":"before"}`)
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	// A user removes the layout override in another editor while the TUI is open.
	if err := os.WriteFile(path, []byte(`{"config_version":1,"extension_setting":"after"}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg.SortColumn = "Assignee"
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	raw := readWorkflowJSON(t, path)
	if _, exists := raw["fields"]; exists {
		t.Fatal("an unrelated save resurrected the externally removed override")
	}
	if string(raw["extension_setting"]) != `"after"` {
		t.Fatal("an unrelated save overwrote an external setting change")
	}
	reloaded, err := config.LoadConfig()
	if err != nil || !slices.Equal(reloaded.Fields, config.DefaultFields) {
		t.Fatalf("next startup did not inherit defaults: %v", err)
	}
}

func TestCopiedConfigurationsKeepIndependentSaveBaselines(t *testing.T) {
	path := workflowConfigPath(t, `{"config_version":1}`)
	original, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	copy := *original
	copy.Theme = "teams"
	if err := config.SaveConfig(&copy); err != nil {
		t.Fatal(err)
	}
	original.SortColumn = "ID"
	if err := config.SaveConfig(original); err != nil {
		t.Fatal(err)
	}
	raw := readWorkflowJSON(t, path)
	if string(raw["theme"]) != `"teams"` || string(raw["sort_column"]) != `"ID"` {
		t.Fatal("saving a shallow copy mutated the original save baseline")
	}
	if _, exists := raw["fields"]; exists {
		t.Fatal("copy saves froze inherited defaults")
	}
}

func TestExplicitUIResetsKeepEmptyValues(t *testing.T) {
	path := workflowConfigPath(t, `{"config_version":1,"gitlab_command":"old-command","activity_filters":["Comments"]}`)
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.GitLabCommand = ""
	cfg.ActivityFilters = []string{}
	if err := config.SaveConfig(cfg, "gitlab_command", "activity_filters"); err != nil {
		t.Fatal(err)
	}
	raw := readWorkflowJSON(t, path)
	if string(raw["gitlab_command"]) != `""` || string(raw["activity_filters"]) != `[]` {
		t.Fatalf("explicit resets omitted or inherited: command=%s activity=%s", raw["gitlab_command"], raw["activity_filters"])
	}
}

func TestDefaultsHaveIndependentCollectionsAndStaySparseOnSave(t *testing.T) {
	path := workflowConfigPath(t, `{"config_version":1}`)
	first, second := config.DefaultConfig(), config.DefaultConfig()
	first.Fields[0] = "Changed"
	first.CustomTypes["default"][0] = "Changed"
	if second.Fields[0] == "Changed" || second.GetCustomTypes("")[0] == "Changed" {
		t.Fatal("default configurations share mutable collections")
	}
	if err := config.SaveConfig(second); err != nil {
		t.Fatal(err)
	}
	raw := readWorkflowJSON(t, path)
	if len(raw) != 1 || string(raw["config_version"]) != "1" {
		t.Fatal("saving fresh defaults created personal overrides")
	}
}

func TestLiteralConfigurationKeepsSuppliedPreferences(t *testing.T) {
	path := workflowConfigPath(t, `{"config_version":1}`)
	cfg := &config.Config{
		URL:            "https://example.test",
		Token:          "$YOUTRACK_TOKEN",
		Fields:         []string{"ID", "Summary"},
		RenderMarkdown: false,
		MaxIssues:      0,
	}
	// Struct literals cannot distinguish an omitted zero from an explicit one;
	// callers declare zero choices while nonzero values remain compatible.
	if err := config.SaveConfig(cfg, "render_markdown", "max_issues"); err != nil {
		t.Fatal(err)
	}
	reloaded, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.URL != cfg.URL || reloaded.Token != cfg.Token || !slices.Equal(reloaded.Fields, cfg.Fields) {
		t.Fatal("supplied literal preferences were discarded")
	}
	if reloaded.RenderMarkdown || reloaded.MaxIssues != 0 || reloaded.PageSize != config.DefaultPageSize {
		t.Fatal("explicit zero choices and omitted defaults were not distinguished")
	}
	if _, exists := readWorkflowJSON(t, path)["page_size"]; exists {
		t.Fatal("omitted literal setting became a personal override")
	}
}
