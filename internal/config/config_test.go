package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func writeTestConfig(t *testing.T, contents string) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	dir := filepath.Join(os.Getenv("HOME"), ".config", "yt-tui")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestNewConfigIncludesTypeColumnWithoutFiltering(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ID", "Type", "Summary", "State", "Priority", "Assignee"}
	if !slices.Equal(cfg.Fields, want) {
		t.Fatalf("default fields = %v, want %v", cfg.Fields, want)
	}
	if !slices.Contains(cfg.GetCustomTypes("APP"), "Story") {
		t.Fatal("default creation types must include Story")
	}
	if len(cfg.GetFilteredTypes("APP")) != 0 {
		t.Fatal("new configurations must not restrict issue types")
	}
}

func TestLegacyConfigPreservesColumnsAndUnrestrictedTypes(t *testing.T) {
	path := writeTestConfig(t, `{"fields":["ID","Summary","Assignee"]}`)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cfg.Fields, []string{"ID", "Summary", "Assignee"}) {
		t.Fatalf("existing columns changed: %v", cfg.Fields)
	}
	if len(cfg.GetFilteredTypes("APP")) != 0 || cfg.GetTypeFieldName("APP") != "" {
		t.Fatal("absent type configuration must use all types and automatic field detection")
	}
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]json.RawMessage
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"filtered_types", "type_field_name"} {
		if _, exists := saved[key]; exists {
			t.Errorf("loading and saving legacy config added %s", key)
		}
	}
}

func TestTypeConfigurationRoundTrip(t *testing.T) {
	writeTestConfig(t, `{
		"filtered_types":{"default":["Task"],"APP":["Bug","Story"],"ALL":[]},
		"type_field_name":{"default":"Category","APP":"Ticket Type"}
	}`)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reloaded.FilteredTypes, cfg.FilteredTypes) ||
		!reflect.DeepEqual(reloaded.TypeFieldName, cfg.TypeFieldName) {
		t.Fatal("type selections or field names changed during persistence")
	}
	for _, tc := range []struct {
		project string
		types   []string
		field   string
	}{
		{"APP", []string{"Bug", "Story"}, "Ticket Type"},
		{"ALL", []string{}, "Category"},
		{"OTHER", []string{"Task"}, "Category"},
		{"", []string{"Task"}, "Category"},
	} {
		t.Run(tc.project, func(t *testing.T) {
			if got := reloaded.GetFilteredTypes(tc.project); !slices.Equal(got, tc.types) {
				t.Errorf("types = %v, want %v", got, tc.types)
			}
			if got := reloaded.GetTypeFieldName(tc.project); got != tc.field {
				t.Errorf("field = %q, want %q", got, tc.field)
			}
		})
	}
	if types, exists := reloaded.FilteredTypes["ALL"]; !exists || types == nil {
		t.Fatal("explicit all-types selection must remain an empty list")
	}
}

func TestSetFilteredTypesPersistsAllOverride(t *testing.T) {
	path := writeTestConfig(t, `{"filtered_types":{"default":["Task"]}}`)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.SetFilteredTypes("APP", nil)
	selection := []string{"Story"}
	cfg.SetFilteredTypes("OTHER", selection)
	selection[0] = "Bug"
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		FilteredTypes map[string]json.RawMessage `json:"filtered_types"`
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if string(saved.FilteredTypes["APP"]) != "[]" {
		t.Fatalf("all types serialized as %s, want []", saved.FilteredTypes["APP"])
	}
	reloaded, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.GetFilteredTypes("APP")) != 0 {
		t.Fatal("explicit all-types selection inherited the default restriction")
	}
	if !slices.Equal(reloaded.GetFilteredTypes("OTHER"), []string{"Story"}) {
		t.Fatal("saved selection was changed by caller-owned slice")
	}
	reloaded.SetFilteredTypes("", []string{"Epic"})
	if !slices.Equal(reloaded.GetFilteredTypes("UNCONFIGURED"), []string{"Epic"}) {
		t.Fatal("empty project code must update the default selection")
	}
}

func TestTypeConfigurationFallbacks(t *testing.T) {
	var absent *Config
	if len(absent.GetFilteredTypes("APP")) != 0 || absent.GetTypeFieldName("APP") != "" {
		t.Fatal("nil config must allow all types and automatic field detection")
	}
	absent.SetFilteredTypes("APP", []string{"Task"})
	cfg := &Config{
		FilteredTypes: map[string][]string{"APP": {"Bug"}},
		TypeFieldName: map[string]string{"APP": "Issue Category"},
	}
	if len(cfg.GetFilteredTypes("OTHER")) != 0 || cfg.GetTypeFieldName("OTHER") != "" {
		t.Fatal("projects without a configured default must use automatic behavior")
	}
	cfg.TypeFieldName["default"] = "Type"
	cfg.TypeFieldName["APP"] = ""
	if cfg.GetTypeFieldName("APP") != "" {
		t.Fatal("explicit empty project field name must restore automatic detection")
	}
}
