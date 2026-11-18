package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	m.tagsLoaded = true
	m.tags = []ytcli.Tag{{ID: "1", Name: "backend"}, {ID: "2", Name: "needs review"}, {ID: "3", Name: "backend"}}
	return m
}

func TestTagsDraftSearchSaveAndSparseDefaults(t *testing.T) {
	m := tagFilterModel(t)
	m.selectedTags = []string{"backend"}
	m.restoreOrLoadIssues()
	m.openFilters()
	m.filterPanel.section = 3
	m.filterPanel.startSearch()
	m.filterPanel.updateSearch(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("review")})
	m.filterPanel.updateSearch(tea.KeyMsg{Type: tea.KeyEnter})
	m.filterPanel.move("down")
	m.filterPanel.toggle()
	m.saveFilters()
	if m.filterMode || !slices.Equal(m.selectedTags, []string{"backend", "needs review"}) {
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
	if len(cfg.FilteredTags) != 0 {
		t.Fatal("tag selection persisted to config")
	}
	if m.serverQuery != "" {
		t.Fatalf("clear left query %q", m.serverQuery)
	}
}

func TestInvalidTagKeepsActiveSelectionAndDraft(t *testing.T) {
	m := tagFilterModel(t)
	m.selectedTags = []string{"backend"}
	m.openFilters()
	m.filterPanel.sections[3].Selected = selection([]string{"invalid*"})
	if cmd := m.saveFilters(); cmd != nil || m.filterError == "" || !m.filterMode || !slices.Equal(m.selectedTags, []string{"backend"}) {
		t.Fatal("invalid tag applied or discarded the draft")
	}
}

func TestTagsCombineWithTypeAndBaseQuery(t *testing.T) {
	m := tagFilterModel(t)
	m.query = "State: Open or Priority: Critical"
	m.selectedTypes = []string{"Epic"}
	m.selectedTags = []string{"backend", "needs review"}
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
	m.selectedTags = []string{"old name"}
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

func TestClearOnlyAffectsCurrentList(t *testing.T) {
	m := tagFilterModel(t)
	m.selectedTags, m.selectedTypes = []string{"backend"}, []string{"Story"}
	m.searchInput.SetValue("first")
	m.excludedStates, m.excludedPriorities = []string{"Closed"}, []string{"Low"}
	m.initContext("", "reporter: me", false)
	if m.searchInput.Value() != "" || len(m.excludedStates)+len(m.excludedPriorities)+len(m.selectedTypes)+len(m.selectedTags) != 0 {
		t.Fatal("project filters leaked into reporter list")
	}
	m.searchInput.SetValue("mine")
	m.clearFilters()
	if m.serverQuery != "reporter: me" {
		t.Fatal("clear discarded reporter constraint")
	}
	m.initProject("DEMO", false)
	if m.searchInput.Value() != "first" || !slices.Equal(m.selectedTags, []string{"backend"}) || !slices.Equal(m.selectedTypes, []string{"Story"}) || !slices.Equal(m.excludedStates, []string{"Closed"}) || !slices.Equal(m.excludedPriorities, []string{"Low"}) {
		t.Fatal("clearing another list lost session filters")
	}
	m.clearFilters()
	m.initContext("", "sprint:board:sprint:Board:Sprint", false)
	m.searchInput.SetValue("mine")
	m.clearFilters()
	if m.agileID != "board" || m.sprintID != "sprint" {
		t.Fatal("clear discarded board/sprint")
	}
	m.initProject("DEMO", false)
	if m.searchInput.Value() != "" || len(m.excludedStates)+len(m.excludedPriorities)+len(m.selectedTypes)+len(m.selectedTags) != 0 {
		t.Fatal("cleared filters returned on navigation")
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
	if len(m.availableTags()) != 2 || tagRequests != 3 || identityRequests != 0 {
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
	// A new process starts with an unrestricted query.
	fresh := newIssuesModel(client, cfg)
	fresh = runIssueCommands(t, fresh, fresh.initProject("0-1", false))
	if fresh.err != nil || fresh.issues[0].IDReadable != "DEMO-1" {
		t.Fatal("new model restored a tag restriction")
	}
	if tagRequests != 3 || identityRequests != 0 {
		t.Fatal("new session requested metadata for a stale filter")
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
	m.selectedTags = []string{"backend"}
	m.searchInput.SetValue("private")
	old := issueTagsMsg{contextKey: m.contextKey, requestID: 3, tags: []ytcli.Tag{{ID: "99", Name: "secret"}}}
	client.SetCredentials("https://example.invalid", "two")
	m, _ = m.Update(old)
	if slices.Contains(m.availableTags(), "secret") {
		t.Fatal("old account metadata accepted")
	}
	m.startContext(false)
	if m.tagsLoaded || len(m.tags) > 0 || len(m.selectedTags) > 0 || m.searchInput.Value() != "" {
		t.Fatal("previous account metadata reused")
	}
	m.openFilters()
	stale := issueTagsMsg{contextKey: m.contextKey, requestID: m.requestID, tags: old.tags}
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
	m, _ = m.Update(issueTagsMsg{contextKey: m.contextKey, requestID: m.requestID, tags: []ytcli.Tag{{ID: "1", Name: "backend"}}})
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

func TestTagCatalogueErrorCanBeRetriedWithinSession(t *testing.T) {
	m := tagFilterModel(t)
	m.selectedTags = []string{"backend"}
	m.restoreOrLoadIssues()
	m.tagsLoaded = false
	m.openFilters()
	failed := issueTagsMsg{contextKey: m.contextKey, requestID: m.requestID, metadataID: m.tagsRequestID, err: errors.New("unavailable")}
	m, _ = m.Update(failed)
	m.openFilters()
	ok := issueTagsMsg{contextKey: m.contextKey, requestID: m.requestID, metadataID: m.tagsRequestID, tags: []ytcli.Tag{{ID: "1", Name: "backend"}}}
	m, _ = m.Update(ok)
	m, _ = m.Update(failed)
	if !m.tagsLoaded || m.tagsErr != nil || m.serverQuery != "tag: {backend}" {
		t.Fatal("stale error replaced current catalogue or changed active query")
	}
}

func TestTagCaseVariantsShareOneSelectedOption(t *testing.T) {
	m := tagFilterModel(t)
	m.tags = append(m.tags, ytcli.Tag{ID: "4", Name: "Backend"})
	m.selectedTags = []string{"BACKEND"}
	m.restoreOrLoadIssues()
	m.openFilters()
	if got := m.filterPanel.sections[3].Options; !slices.Equal(got, []string{"BACKEND", "needs review"}) {
		t.Fatalf("case variants created duplicate/lost checkbox: %v", got)
	}
	if strings.Contains(m.tagNotice(), "unavailable") {
		t.Fatal("case variant incorrectly marked unavailable")
	}
}
