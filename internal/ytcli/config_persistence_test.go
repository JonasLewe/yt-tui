package ytcli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"yt-tui/internal/config"
)

func readCredentialOverrides(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".config", "yt-tui", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"fields", "page_size", "custom_types", "theme", "render_markdown"} {
		if _, exists := values[key]; exists {
			t.Errorf("saving credentials froze inherited setting %q", key)
		}
	}
	return values
}

func TestSaveCredentialsPersistsReferencesWithoutFreezingDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("YT_CONFIG_TEST_URL", "https://example.invalid")
	t.Setenv("YT_CONFIG_TEST_TOKEN", "test-token")
	client := &Client{}
	if err := client.SaveCredentials("$YT_CONFIG_TEST_URL", "$YT_CONFIG_TEST_TOKEN"); err != nil {
		t.Fatal(err)
	}
	if client.baseURL != "https://example.invalid" || client.token != "test-token" {
		t.Fatal("credentials were not resolved for the active client")
	}
	values := readCredentialOverrides(t)
	var baseURL, token string
	if err := json.Unmarshal(values["url"], &baseURL); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(values["token"], &token); err != nil {
		t.Fatal(err)
	}
	if baseURL != "$YT_CONFIG_TEST_URL" || token != "$YT_CONFIG_TEST_TOKEN" {
		t.Fatal("saving credentials replaced their environment references")
	}
	if err := client.SaveCredentials("", ""); err != nil {
		t.Fatal(err)
	}
	values = readCredentialOverrides(t)
	if string(values["url"]) != `""` || string(values["token"]) != `""` {
		t.Fatal("explicit credential clearing was omitted")
	}
}

func TestLegacyCredentialsMigrationKeepsConfigurationSparse(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("YOUTRACK_BASE_URL", "")
	t.Setenv("YOUTRACK_TOKEN", "")
	t.Chdir(t.TempDir())
	dir := filepath.Join(os.Getenv("HOME"), ".config", "youtrack-cli")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("YOUTRACK_BASE_URL=https://example.invalid\nYOUTRACK_TOKEN=legacy-test-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	client := NewClient()
	if client.baseURL != "https://example.invalid" || client.token != "legacy-test-token" {
		t.Fatal("legacy credentials did not initialize the client")
	}
	readCredentialOverrides(t)
	reloaded, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.URL != client.baseURL || reloaded.Token != client.token {
		t.Fatal("legacy credentials were not persisted for subsequent startups")
	}
}
