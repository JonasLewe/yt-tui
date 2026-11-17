package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func persistedJSON(t *testing.T, path string) configDocument {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc configDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestLegacySnapshotMigrationHasExactBackupAndIsIdempotent(t *testing.T) {
	input := `{
  "fields": ["ID", "Summary", "State", "Priority", "Assignee"],
  "url": "https://example.test", "token": "$YOUTRACK_TOKEN",
  "custom_types": {"default":["Bug","Feature","Task","Epic","Improvement","Support"], "APP":["Initiative"]},
  "custom_states": {"default":["Open","In Progress","Verified","Done","Duplicate","Won't fix","Incomplete"]},
  "filtered_states": {"default":["Done","Open","In Progress","Verified","Duplicate","Won't fix","Incomplete"], "APP":[]},
  "render_markdown": true, "max_issues":500, "page_size":20,
  "activity_filters":["Comments"], "theme":"catppuccin",
  "extension_setting":{"keep":"exact semantic value"}
}`
	path := writeTestConfig(t, input)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cfg.Fields, DefaultFields) || !slices.Contains(cfg.GetCustomTypes(""), "Story") {
		t.Fatal("old defaults did not advance")
	}
	if !slices.Equal(cfg.GetCustomTypes("APP"), []string{"Initiative"}) || len(cfg.GetFilteredStates("APP")) != 0 {
		t.Fatal("personal values changed")
	}
	doc := persistedJSON(t, path)
	for _, key := range []string{"fields", "render_markdown", "max_issues", "page_size", "activity_filters", "theme", "custom_states"} {
		if _, exists := doc[key]; exists {
			t.Errorf("generated default %s still pinned", key)
		}
	}
	if string(doc["token"]) != `"$YOUTRACK_TOKEN"` || string(doc["config_version"]) != "1" {
		t.Fatal("credential/version changed incorrectly")
	}
	for _, key := range []string{"custom_types", "filtered_states"} {
		object, _ := jsonObject(doc[key])
		if _, exists := object["default"]; exists {
			t.Errorf("generated %s default still pinned", key)
		}
	}
	backups, err := filepath.Glob(filepath.Join(filepath.Dir(path), "config.json.before-v1-*"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups=%v error=%v", backups, err)
	}
	data, err := os.ReadFile(backups[0])
	if err != nil || string(data) != input {
		t.Fatal("backup does not contain exact original bytes")
	}
	for _, name := range []string{path, backups[0]} {
		info, err := os.Stat(name)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("insecure permissions on %s", name)
		}
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	backups, _ = filepath.Glob(filepath.Join(filepath.Dir(path), "config.json.before-v1-*"))
	if string(before) != string(after) || len(backups) != 1 {
		t.Fatal("migration repeated on next startup")
	}
}

func TestVersionedKnownDefaultsAreExplicit(t *testing.T) {
	path := writeTestConfig(t, `{"config_version":1,"fields":["ID","Summary","State","Priority","Assignee"],"page_size":20,"render_markdown":true}`)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(cfg.Fields, "Type") {
		t.Fatal("versioned custom layout was migrated")
	}
	cfg.SortColumn = "ID"
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	doc := persistedJSON(t, path)
	for _, key := range []string{"fields", "page_size", "render_markdown"} {
		if _, ok := doc[key]; !ok {
			t.Fatalf("explicit %s removed", key)
		}
	}
	backups, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "config.json.before-v1-*"))
	if len(backups) != 0 {
		t.Fatal("versioned file was backed up/migrated")
	}
}

func TestNullInheritsWithoutReplacingExplicitEmptyValues(t *testing.T) {
	path := writeTestConfig(t, `{"config_version":1,"fields":null,"render_markdown":false,"max_issues":0,"activity_filters":[],"custom_types":{"default":[],"APP":null},"type_field_name":{"default":"Category","APP":""},"boards_field_name":{"default":"Boards","APP":""},"actions":{"default":[{"name":"Ship","commands":[]}],"APP":[]}}`)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cfg.Fields, DefaultFields) || cfg.RenderMarkdown || cfg.MaxIssues != 0 || len(cfg.ActivityFilters) != 0 {
		t.Fatal("null/false/zero/empty override semantics incorrect")
	}
	if len(cfg.GetCustomTypes("")) != 0 || len(cfg.GetActions("APP")) != 0 || len(cfg.GetActions("OTHER")) != 1 {
		t.Fatal("explicit empty list did not override default choices")
	}
	if cfg.GetTypeFieldName("APP") != "" || cfg.GetBoardsFieldName("APP") != "" {
		t.Fatal("explicit automatic field detection ignored")
	}
	cfg.SortColumn = "ID"
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if string(persistedJSON(t, path)["fields"]) != "null" {
		t.Fatal("inherited null changed to pinned fields")
	}
}

func TestInvalidOrFutureConfigCannotBeOverwrittenThroughFallback(t *testing.T) {
	for _, input := range []string{`{`, `[]`, `null`, `{"config_version":2,"token":"keep"}`, `{"config_version":1,"page_size":0}`, `{"config_version":1,"max_issues":-1}`} {
		t.Run(input, func(t *testing.T) {
			path := writeTestConfig(t, input)
			cfg, err := LoadConfig()
			if err == nil {
				t.Fatal("invalid config accepted")
			}
			cfg.SortColumn = "ID"
			if err := SaveConfig(cfg, "sort_column"); err == nil {
				t.Fatal("fallback overwrote invalid file")
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != input {
				t.Fatal("invalid original was changed")
			}
			backups, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "config.json.before-v1-*"))
			if len(backups) != 0 {
				t.Fatal("invalid file migrated")
			}
		})
	}
}

func TestConfigFileSymlinkRetainedDuringMigrationAndSaves(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(os.Getenv("HOME"), ".config", "yt-tui", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "preferences.json")
	original := `{"fields":["ID","Summary","State","Priority","Assignee"],"token":"$YOUTRACK_TOKEN"}`
	if err := os.WriteFile(target, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Theme = "teams"
	if err := SaveConfig(cfg, "theme"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("config symlink replaced")
	}
	doc := persistedJSON(t, target)
	if string(doc["theme"]) != `"teams"` {
		t.Fatal("symlink target not saved")
	}
	backups, _ := filepath.Glob(filepath.Join(filepath.Dir(target), "config.json.before-v1-*"))
	if len(backups) != 1 {
		t.Fatal("target backup missing")
	}
	data, _ := os.ReadFile(backups[0])
	if string(data) != original {
		t.Fatal("target backup changed original")
	}
}

func TestDanglingConfigSymlinkIsNotReplaced(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(os.Getenv("HOME"), ".config", "yt-tui", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing.json", path); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err == nil {
		t.Fatal("dangling link accepted")
	}
	if err := SaveConfig(cfg); err == nil {
		t.Fatal("dangling link overwritten")
	}
	link, err := os.Readlink(path)
	if err != nil || link != "missing.json" {
		t.Fatal("link changed")
	}
}

func TestFailedAtomicWriteLeavesOriginalAndTrackingUntouched(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission test requires an unprivileged user")
	}
	path := writeTestConfig(t, `{"config_version":1,"token":"keep"}`)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	tracking := cfg.tracking
	cfg.SetFilteredTypes("APP", []string{"Epic"})
	if err := os.Chmod(filepath.Dir(path), 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(path), 0700) })
	if err := SaveConfig(cfg); err == nil {
		t.Fatal("save unexpectedly succeeded")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) || cfg.tracking != tracking || len(cfg.explicitPaths) == 0 {
		t.Fatal("failed save changed disk or consumed pending intent")
	}
	if err := os.Chmod(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadConfig()
	if err != nil || !slices.Equal(reloaded.GetFilteredTypes("APP"), []string{"Epic"}) {
		t.Fatal("failed save could not be retried")
	}
}

func TestLegacyBackupFailureDoesNotStartMigration(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission test requires an unprivileged user")
	}
	input := `{"fields":["ID","Summary","State","Priority","Assignee"]}`
	path := writeTestConfig(t, input)
	if err := os.Chmod(filepath.Dir(path), 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(path), 0700) })
	if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "back up") {
		t.Fatalf("expected backup failure, got %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != input {
		t.Fatal("original modified without backup")
	}
}

func TestNestedOverrideRemovalReturnsCurrentDefault(t *testing.T) {
	path := writeTestConfig(t, `{"config_version":1,"custom_types":{"default":["Custom"],"APP":["Epic"]}}`)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	delete(cfg.CustomTypes, "default")
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	doc := persistedJSON(t, path)
	values, _ := jsonObject(doc["custom_types"])
	if _, exists := values["default"]; exists {
		t.Fatal("removed map override reappeared")
	}
	reloaded, err := LoadConfig()
	if err != nil || !slices.Equal(reloaded.GetCustomTypes(""), DefaultTypes) || !slices.Equal(reloaded.GetCustomTypes("APP"), []string{"Epic"}) {
		t.Fatal("map override reset did not inherit defaults")
	}
}

func TestPendingSetterIntentDoesNotLeakAcrossShallowCopies(t *testing.T) {
	path := writeTestConfig(t, `{"config_version":1}`)
	original, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	original.SetFilteredTypes("ONE", []string{"Epic"})
	draft := *original
	draft.FilteredTypes = map[string][]string{"ONE": {"Epic"}}
	draft.SetFilteredTypes("TWO", []string{})
	if len(original.explicitPaths) != 1 || len(draft.explicitPaths) != 2 {
		t.Fatal("draft mutated original pending paths")
	}
	if err := SaveConfig(original); err != nil {
		t.Fatal(err)
	}
	values, _ := jsonObject(persistedJSON(t, path)["filtered_types"])
	if _, exists := values["TWO"]; exists {
		t.Fatal("unsaved draft leaked into original save")
	}
}

func TestFactoryConfigPersistsDeliberateZeroChanges(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := DefaultConfig()
	cfg.RenderMarkdown = false
	cfg.MaxIssues = 0
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(os.Getenv("HOME"), ".config", "yt-tui", "config.json")
	doc := persistedJSON(t, path)
	if string(doc["render_markdown"]) != "false" || string(doc["max_issues"]) != "0" {
		t.Fatal("factory-created config discarded deliberately changed zero values")
	}
	if len(doc) != 3 {
		t.Fatalf("factory save froze untouched defaults: got %d keys, want 3", len(doc))
	}
	reloaded, err := LoadConfig()
	if err != nil || reloaded.RenderMarkdown || reloaded.MaxIssues != 0 || !slices.Equal(reloaded.Fields, DefaultFields) {
		t.Fatalf("factory changes or inherited defaults lost on reload: %v", err)
	}
}
