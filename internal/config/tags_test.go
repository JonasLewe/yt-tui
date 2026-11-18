package config

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

func TestTagFiltersScopeAliasesEmptyAndNull(t *testing.T) {
	path := writeTestConfig(t, `{
		"config_version":1,
		"filtered_tags":{
			"https://one.test#1-1":{"default":["backend"],"APP":["needs review"],"0-1":["by ID"],"ALL":[],"NULL":null},
			"https://one.test#1-2":{"APP":["other account"]},
			"https://two.test#1-1":{"APP":["other server"]}
		}
	}`)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		scope   string
		aliases []string
		want    []string
	}{
		{"https://one.test#1-1", []string{"APP", "0-1"}, []string{"needs review"}},
		{"https://one.test#1-1", []string{"RENAMED", "0-1"}, []string{"by ID"}},
		{"https://one.test#1-1", []string{"OTHER"}, []string{"backend"}},
		{"https://one.test#1-1", []string{"ALL", "0-1"}, []string{}},
		{"https://one.test#1-1", []string{"NULL"}, []string{"backend"}},
		{"https://one.test#1-1", nil, []string{"backend"}},
		{"https://one.test#1-2", []string{"APP"}, []string{"other account"}},
		{"https://two.test#1-1", []string{"APP"}, []string{"other server"}},
		{"https://one.test#1-3", []string{"APP"}, nil},
		{"", []string{"APP"}, nil},
	} {
		if got := cfg.GetFilteredTags(tc.scope, tc.aliases...); !slices.Equal(got, tc.want) {
			t.Errorf("scope=%q aliases=%v: got %v, want %v", tc.scope, tc.aliases, got, tc.want)
		}
	}
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	doc := persistedJSON(t, path)
	if len(doc) != 2 || string(doc["config_version"]) != "1" {
		t.Fatal("loading/saving tags pinned unrelated defaults or changed schema")
	}
}

func TestSetFilteredTagsSparseRoundTripAndCopyOnWrite(t *testing.T) {
	path := writeTestConfig(t, `{"config_version":1,"filtered_tags":{"server#account":{"default":["backend"],"APP":["old"]}},"favorite_views":{"server":"APP"}}`)
	original, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	draft := *original
	draft.SetFilteredTags("server#account", "APP", nil)
	names := []string{"frontend"}
	draft.SetFilteredTags("server#account", "OTHER", names)
	names[0] = "caller mutation"
	if !slices.Equal(original.GetFilteredTags("server#account", "APP"), []string{"old"}) || len(original.explicitPaths) != 0 {
		t.Fatal("draft changed original config or override tracking")
	}
	read := draft.GetFilteredTags("server#account", "OTHER")
	read[0] = "getter mutation"
	if err := SaveConfig(&draft); err != nil {
		t.Fatal(err)
	}
	doc := persistedJSON(t, path)
	if len(doc) != 3 {
		t.Fatalf("tag save froze unrelated defaults: %v", doc)
	}
	var saved map[string]map[string]json.RawMessage
	if err := json.Unmarshal(doc["filtered_tags"], &saved); err != nil {
		t.Fatal(err)
	}
	if string(saved["server#account"]["APP"]) != "[]" {
		t.Fatal("explicit all-tags override must serialize as []")
	}
	reloaded, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.GetFilteredTags("server#account", "APP")) != 0 || !slices.Equal(reloaded.GetFilteredTags("server#account", "OTHER"), []string{"frontend"}) {
		t.Fatal("selection changed during save or through caller-owned slices")
	}
	if !jsonEqual(doc["favorite_views"], json.RawMessage(`{"server":"APP"}`)) {
		t.Fatal("disabled favorites data must remain intact")
	}
	// A new account's default is explicit, even when it equals today's all-tags
	// default, and cannot leak into another account.
	reloaded.SetFilteredTags("other#account", "", nil)
	if err := SaveConfig(reloaded); err != nil {
		t.Fatal(err)
	}
	doc = persistedJSON(t, path)
	json.Unmarshal(doc["filtered_tags"], &saved)
	if string(saved["other#account"]["default"]) != "[]" {
		t.Fatal("explicit default intent was lost")
	}
}

func TestTagSavePreservesConcurrentUnrelatedChanges(t *testing.T) {
	path := writeTestConfig(t, `{"config_version":1,"filtered_tags":{"server#account":{"APP":["old"]}}}`)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"config_version":1,"filtered_tags":{"server#account":{"APP":["old"],"MANUAL":["keep"]},"another#account":{"APP":["other"]}},"extension":{"keep":true}}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg.SetFilteredTags("server#account", "APP", []string{"new"})
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(reloaded.GetFilteredTags("server#account", "MANUAL"), []string{"keep"}) || !slices.Equal(reloaded.GetFilteredTags("another#account", "APP"), []string{"other"}) {
		t.Fatal("saving one project's tags overwrote unrelated external changes")
	}
	if _, ok := persistedJSON(t, path)["extension"]; !ok {
		t.Fatal("unknown config extension was removed")
	}
}

func TestTagDefaultsRemainUnrestrictedAndSparse(t *testing.T) {
	path := writeTestConfig(t, `{"config_version":1}`)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.GetFilteredTags("server#account", "APP")) != 0 {
		t.Fatal("new configs must not filter tags")
	}
	cfg.SetFilteredTags("", "APP", []string{"unsafe account fallback"})
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if _, pinned := persistedJSON(t, path)["filtered_tags"]; pinned {
		t.Fatal("unresolved scope or inherited defaults persisted a filter")
	}
	var absent *Config
	if absent.GetFilteredTags("server#account", "APP") != nil {
		t.Fatal("nil config must be unrestricted")
	}
	absent.SetFilteredTags("server#account", "APP", []string{"backend"})
}
