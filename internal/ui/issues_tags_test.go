package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"yt-tui/internal/config"
	"yt-tui/internal/ytcli"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func tagFilterModel(t *testing.T) issuesModel {
	t.Helper()
	m := typeFilterModel(t)
	m.tagScope, m.tagScopeReady, m.tagsLoaded = "https://example.invalid#user-1", true, true
	m.tags = []ytcli.Tag{{ID: "1", Name: "backend"}, {ID: "2", Name: "needs review"}, {ID: "3", Name: "backend"}}
	return m
}

func TestTagsDraftSearchSaveAndSparseDefaults(t *testing.T) {
	m := tagFilterModel(t)
	m.cfg.SetFilteredTags(m.tagScope, "default", []string{"backend"})
	m.restoreOrLoadIssues()
	m.openFilters()
	m.filterPanel.section = 3
	m.filterPanel.startSearch()
	m.filterPanel.updateSearch(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("review")})
	m.filterPanel.updateSearch(tea.KeyMsg{Type: tea.KeyEnter})
	m.filterPanel.move("down")
	m.filterPanel.toggle()
	m.saveFilters()
	if m.filterMode || !slices.Equal(m.cfg.GetFilteredTags(m.tagScope, "DEMO"), []string{"backend", "needs review"}) {
		t.Fatalf("selection failed: %v / %s", m.selectedTags, m.filterError)
	}
	if !strings.Contains(m.serverQuery, "tag: {backend}, {needs review}") {
		t.Fatalf("wrong query: %s", m.serverQuery)
	}
	if _, ok := m.cfg.FilteredStates["DEMO"]; ok {
		t.Fatal("tag edit pinned state defaults")
	}
	if _, ok := m.cfg.FilteredTypes["DEMO"]; ok {
		t.Fatal("tag edit pinned type defaults")
	}
	m.openFilters()
	m.filterPanel.section, m.filterPanel.cursor = 3, 0
	m.filterPanel.toggle()
	m.saveFilters()
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if names, ok := cfg.FilteredTags[m.tagScope]["DEMO"]; !ok || len(names) != 0 {
		t.Fatal("No restriction must explicitly override inherited tags")
	}
	if m.serverQuery != "" {
		t.Fatalf("clear left query %q", m.serverQuery)
	}
}

func TestTagsSaveFailureKeepsAliasesAndDraft(t *testing.T) {
	m := tagFilterModel(t)
	m.cfg.SetFilteredTags(m.tagScope, "0-1", []string{"backend"})
	m.restoreOrLoadIssues()
	m.openFilters()
	m.filterPanel.sections[3].Selected = selection([]string{"needs review"})
	path := filepath.Join(os.Getenv("HOME"), ".config", "yt-tui", "config.json")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if cmd := m.saveFilters(); cmd != nil || m.filterError == "" || !m.filterMode {
		t.Fatal("failed save applied draft")
	}
	if !slices.Equal(m.cfg.GetFilteredTags(m.tagScope, "0-1"), []string{"backend"}) {
		t.Fatal("failed save changed source alias")
	}
	if _, ok := m.cfg.FilteredTags[m.tagScope]["DEMO"]; ok {
		t.Fatal("failed save added canonical entry")
	}
}

func TestTagsCombineWithTypeAndBaseQuery(t *testing.T) {
	m := tagFilterModel(t)
	m.query = "State: Open or Priority: Critical"
	m.cfg.SetFilteredTypes("DEMO", []string{"Epic"})
	m.cfg.SetFilteredTags(m.tagScope, "0-1", []string{"needs review", "backend"})
	m.restoreOrLoadIssues()
	want := "((State: Open or Priority: Critical) and (Type: {Epic})) and (tag: {backend}, {needs review})"
	if m.serverQuery != want {
		t.Fatalf("query=%s", m.serverQuery)
	}
	if !strings.Contains(m.filterSummary(), "Tags (any): backend, needs review") {
		t.Fatal("active tag restriction not visible")
	}
}

func TestTagCatalogueFailuresAndUnavailableNamesRetainRestriction(t *testing.T) {
	m := tagFilterModel(t)
	m.cfg.SetFilteredTags(m.tagScope, "DEMO", []string{"old name"})
	m.tagsErr = errors.New("forbidden")
	m.restoreOrLoadIssues()
	if m.serverQuery != "tag: {old name}" || m.err != nil {
		t.Fatalf("catalogue error lost filter: %q %v", m.serverQuery, m.err)
	}
	m.openFilters()
	if !m.filterPanel.sections[3].Selected["old name"] || !strings.Contains(m.tagNotice(), "retained") {
		t.Fatal("missing saved tag disappeared")
	}
	m.tagsErr = nil
	if !strings.Contains(m.tagNotice(), "unavailable") {
		t.Fatal("unavailable saved tag not explained")
	}
}

func TestTagAccountFailureBlocksSavedFilters(t *testing.T) {
	m := tagFilterModel(t)
	m.cfg.SetFilteredTags(m.tagScope, "DEMO", []string{"backend"})
	m.tagScopeReady = false
	m.tagScopeErr = errors.New("cannot identify user")
	if cmd := m.restoreOrLoadIssues(); cmd != nil || m.err == nil || m.loading {
		t.Fatal("identity failure silently broadened query")
	}
}

func TestTagUnresolvedProjectAliasRequiresExplicitClear(t *testing.T) {
	m := tagFilterModel(t)
	m.cfg.SetFilteredTags(m.tagScope, "0-1", []string{"backend"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/admin/projects":
			http.Error(w, "forbidden", http.StatusForbidden)
		case "/api/issues":
			fmt.Fprint(w, "[]")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	m.client = ytcli.NewClient()
	m.client.SetCredentials(server.URL, "test-token")
	m.contextKey = m.client.ContextKey()
	cmd := m.initProject("DEMO", false)
	m = runIssueCommands(t, m, cmd)
	if !m.unresolvedSavedTagFilter() || m.err == nil {
		t.Fatal("unresolved alias widened filter")
	}
	m.openFilters()
	m.filterPanel.toggle() // Edit only the State section.
	cmd = m.saveFilters()
	m = runIssueCommands(t, m, cmd)
	if _, ok := m.cfg.FilteredTags[m.tagScope]["DEMO"]; ok || !m.unresolvedSavedTagFilter() {
		t.Fatal("saving an unrelated change removed the unresolved tag restriction")
	}
	m.openFilters()
	m.filterPanel.section, m.filterPanel.cursor = 3, 0
	m.filterPanel.toggle()
	cmd = m.saveFilters()
	m = runIssueCommands(t, m, cmd)
	if m.err != nil || !m.loadedAll {
		t.Fatalf("No restriction did not recover the issue list: %v", m.err)
	}
	if names, ok := m.cfg.FilteredTags[m.tagScope]["DEMO"]; !ok || len(names) > 0 {
		t.Fatal("clear wasn't saved")
	}
}

func TestTagsFilterBeforeLimitWithLazyCompleteCatalogue(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var queries []string
	var tagRequests, identityRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/admin/projects/0-1":
			fmt.Fprint(w, `{"id":"0-1","shortName":"DEMO"}`)
		case "/api/admin/projects/0-1/customFields":
			fmt.Fprint(w, `[]`)
		case "/api/users/me":
			identityRequests++
			fmt.Fprint(w, `{"id":"user-1","login":"test"}`)
		case "/api/tags":
			tagRequests++
			switch r.URL.Query().Get("$skip") {
			case "0":
				fmt.Fprint(w, `[{"id":"1","name":"backend"}]`)
			case "1":
				fmt.Fprint(w, `[{"id":"2","name":"needs review"}]`)
			default:
				fmt.Fprint(w, `[]`)
			}
		case "/api/issues":
			query := r.URL.Query().Get("query")
			queries = append(queries, query)
			skip, _ := strconv.Atoi(r.URL.Query().Get("$skip"))
			top, _ := strconv.Atoi(r.URL.Query().Get("$top"))
			first := 1
			if strings.Contains(query, "tag: {backend}, {needs review}") {
				first = 501
			}
			issues := []ytcli.Issue{typedIssue(fmt.Sprint(first), ""), typedIssue(fmt.Sprint(first+1), ""), typedIssue(fmt.Sprint(first+2), "")}
			if skip >= len(issues) {
				fmt.Fprint(w, `[]`)
				return
			}
			json.NewEncoder(w).Encode(issues[skip:min(skip+top, len(issues))])
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.PageSize, cfg.MaxIssues = 2, 2
	client := ytcli.NewClient()
	client.SetCredentials(server.URL, "token")
	m := newIssuesModel(client, cfg)
	m = runIssueCommands(t, m, m.initProject("0-1", false))
	if identityRequests != 0 || tagRequests != 0 {
		t.Fatal("initial unfiltered browsing waited for tag metadata")
	}
	m = runIssueCommands(t, m, m.openFilters())
	if len(m.availableTags()) != 2 || tagRequests != 3 || identityRequests != 1 {
		t.Fatalf("catalogue incomplete: %v requests %d/%d", m.availableTags(), tagRequests, identityRequests)
	}
	m.filterPanel.sections[3].Selected = selection([]string{"backend", "needs review"})
	m = runIssueCommands(t, m, m.saveFilters())
	if m.err != nil || len(m.issues) != 2 || m.issues[0].IDReadable != "DEMO-501" || !m.limitReached {
		t.Fatalf("tag search not applied before cap: %+v err %v", m.issues, m.err)
	}
	if !strings.Contains(queries[len(queries)-1], "tag: {backend}, {needs review}") {
		t.Fatal("server query missing tags")
	}
	// A fresh model restores the account-scoped filter before requesting issues.
	fresh := newIssuesModel(client, cfg)
	fresh = runIssueCommands(t, fresh, fresh.initProject("0-1", false))
	if fresh.err != nil || fresh.issues[0].IDReadable != "DEMO-501" {
		t.Fatal("saved account filter not restored")
	}
	if tagRequests != 3 || identityRequests != 2 {
		t.Fatal("restoring named filter should only resolve account, not catalogue")
	}
}

func TestTagsStaleRequestsAndSameServerAccountChanges(t *testing.T) {
	m := tagFilterModel(t)
	client := ytcli.NewClient()
	client.SetCredentials("https://example.invalid", "one")
	m.client = client
	m.contextKey = client.ContextKey()
	m.serverURL = client.GetConfiguredBaseURL()
	m.requestID = 3
	old := issueTagsMsg{contextKey: m.contextKey, requestID: 3, scope: m.tagScope, withTags: true, tags: []ytcli.Tag{{ID: "99", Name: "secret"}}}
	client.SetCredentials("https://example.invalid", "two")
	m, _ = m.Update(old)
	if slices.Contains(m.availableTags(), "secret") {
		t.Fatal("old account metadata accepted")
	}
	m.startContext(false)
	if m.tagScopeReady || m.tagsLoaded || len(m.tags) > 0 {
		t.Fatal("previous account metadata reused")
	}
	m.openFilters()
	stale := issueTagsMsg{contextKey: m.contextKey, requestID: m.requestID, scope: "other", withTags: true, tags: old.tags}
	m.initProject("OTHER", false)
	if m.tagsLoading {
		t.Fatal("stale pending catalogue survived context transition")
	}
	m, _ = m.Update(stale)
	if slices.Contains(m.availableTags(), "secret") {
		t.Fatal("old project generation metadata accepted")
	}
	if cmd := m.openFilters(); cmd == nil || !m.tagsLoading {
		t.Fatal("new context cannot reload cancelled catalogue")
	}
}

func TestTagCatalogueRefreshPreservesSearchAndDraft(t *testing.T) {
	m := tagFilterModel(t)
	m.openFilters()
	m.filterPanel.section = 3
	m.filterPanel.sections[3].Selected = selection([]string{"needs review"})
	m.filterPanel.sections[3].Search = "review"
	m, _ = m.Update(issueTagsMsg{contextKey: m.contextKey, requestID: m.requestID, scope: m.tagScope, withTags: true, tags: []ytcli.Tag{{ID: "1", Name: "backend"}}})
	section := m.filterPanel.sections[3]
	if !section.Selected["needs review"] || section.Search != "review" || !slices.Contains(section.Options, "needs review") {
		t.Fatal("metadata refresh lost draft/search")
	}
	for _, size := range [][2]int{{1, 1}, {12, 4}, {36, 15}, {80, 24}, {110, 32}} {
		m.width, m.height = size[0], size[1]
		view := m.filterPanelView()
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("tags overflow %v", size)
		}
	}
}

func TestDetailTagsWrapWithoutLosingNames(t *testing.T) {
	issue := ytcli.Issue{Tags: []ytcli.Tag{{Name: "backend"}, {Name: "needs review"}, {Name: "開発"}}}
	for _, width := range []int{1, 10, 30, 80} {
		view := detailTags(issue, width)
		if lipgloss.Width(view) > max(2, width) {
			t.Fatalf("tags exceed width %d", width)
		}
		text := strings.ReplaceAll(strings.ReplaceAll(ansi.Strip(view), "\n", ""), " ", "")
		for _, name := range []string{"backend", "needs review", "開発"} {
			if !strings.Contains(text, strings.ReplaceAll(name, " ", "")) {
				t.Fatalf("lost name %q in %q", name, text)
			}
		}
	}
	if !strings.Contains(detailTags(ytcli.Issue{}, 30), "Tags: None") {
		t.Fatal("empty issue tags not clear")
	}
}

// Opening F while startup resolves the account supersedes the old scope-only
// request. Both delivery orders must keep the latest identity/error state.
func TestTagStartupAndCatalogueOverlapIgnoreOlderScope(t *testing.T) {
	for _, olderFirst := range []bool{true, false} {
		t.Run(fmt.Sprint(olderFirst), func(t *testing.T) {
			m := tagFilterModel(t)
			m.cfg.SetFilteredTags(m.tagScope, "DEMO", []string{"backend"})
			scope := m.tagScope
			m.tagScopeReady = false
			m.startContext(false)
			old := issueTagsMsg{contextKey: m.contextKey, requestID: m.requestID, metadataID: m.tagsRequestID, scopeErr: errors.New("old identity failure")}
			m.tagsLoaded = false
			m.openFilters()
			latest := issueTagsMsg{contextKey: m.contextKey, requestID: m.requestID, metadataID: m.tagsRequestID, scope: scope, withTags: true, tags: []ytcli.Tag{{ID: "1", Name: "backend"}}}
			if old.metadataID == latest.metadataID {
				t.Fatal("overlapping fetches share result identity")
			}
			if olderFirst {
				m, _ = m.Update(old)
			}
			var cmd tea.Cmd
			m, cmd = m.Update(latest)
			if cmd == nil || m.err != nil || !m.tagScopeReady || m.tagScopeLoading || m.tagsLoading || m.serverQuery != "tag: {backend}" {
				t.Fatalf("latest scope did not restore query: err %v query %q", m.err, m.serverQuery)
			}
			if !olderFirst {
				m, _ = m.Update(old)
			}
			if m.err != nil || !m.tagScopeReady || m.tagsErr != nil {
				t.Fatal("late old failure replaced current account")
			}
		})
	}
}

func TestTagIdentityRetryClearsPreviousError(t *testing.T) {
	m := tagFilterModel(t)
	m.cfg.SetFilteredTags(m.tagScope, "DEMO", []string{"backend"})
	scope := m.tagScope
	m.tagScopeReady = false
	m.tagScopeErr = errors.New("old failure")
	m.restoreOrLoadIssues()
	m.tagsLoaded = false
	m.openFilters()
	msg := issueTagsMsg{contextKey: m.contextKey, requestID: m.requestID, metadataID: m.tagsRequestID, scope: scope, withTags: true, err: errors.New("catalogue unavailable")}
	var cmd tea.Cmd
	m, cmd = m.Update(msg)
	if cmd == nil || m.err != nil || m.tagsErr == nil || m.serverQuery != "tag: {backend}" {
		t.Fatal("identity retry must recover constrained browsing despite separate catalogue error")
	}
}

func TestTagCaseVariantsShareOneSelectedOption(t *testing.T) {
	m := tagFilterModel(t)
	m.tags = append(m.tags, ytcli.Tag{ID: "4", Name: "Backend"})
	m.cfg.SetFilteredTags(m.tagScope, "DEMO", []string{"BACKEND"})
	m.restoreOrLoadIssues()
	m.openFilters()
	if got := m.filterPanel.sections[3].Options; !slices.Equal(got, []string{"BACKEND", "needs review"}) {
		t.Fatalf("case variants created duplicate/lost checkbox: %v", got)
	}
	if strings.Contains(m.tagNotice(), "unavailable") {
		t.Fatal("case variant incorrectly marked unavailable")
	}
}
