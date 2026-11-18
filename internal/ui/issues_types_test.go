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
)

func typedIssue(id, kind string) ytcli.Issue {
	i := ytcli.Issue{ID: id, IDReadable: "DEMO-" + id, Summary: "Login " + id,
		Project: &ytcli.Project{ID: "0-1", ShortName: "DEMO"}}
	if kind != "" {
		i.CustomFields = []ytcli.CustomField{{Name: "Type", Value: map[string]interface{}{"name": kind}}}
	}
	return i
}

func typeFilterModel(t *testing.T) issuesModel {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	m := newIssuesModel(nil, cfg)
	m.projectCode, m.width, m.height, m.loading = "DEMO", 110, 24, false
	m.typeInfo = ytcli.IssueTypeInfo{Project: ytcli.Project{ID: "0-1", ShortName: "DEMO"}, Found: true, FieldName: "Type", Values: []string{"Epic", "Story", "Task"}}
	m.typeInfoCache[m.typeKey()] = m.typeInfo
	return m
}

func runIssueCommands(t *testing.T, m issuesModel, cmd tea.Cmd) issuesModel {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 50 {
			t.Fatal("issue commands did not settle")
		}
		cmd, queue = queue[0], queue[1:]
		if cmd == nil {
			continue
		}
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		var next tea.Cmd
		m, next = m.Update(msg)
		if next != nil {
			queue = append(queue, next)
		}
	}
	return m
}

func TestIssueTypesUseRealValuesAndPreserveCustomColumns(t *testing.T) {
	m := typeFilterModel(t)
	m.issues = []ytcli.Issue{typedIssue("1", "Epic"), typedIssue("2", "Story"), typedIssue("3", "")}
	m.updateTableRows()
	for idx, want := range []string{"Epic", "Story", "—"} {
		if got := m.table.Rows()[idx][1]; got != want {
			t.Fatalf("type = %q, want %q", got, want)
		}
	}
	m.cfg.Fields = []string{"ID", "Summary"}
	custom := newIssuesModel(nil, m.cfg)
	if len(custom.fields) != 2 {
		t.Fatal("custom layout overwritten")
	}
	m.cfg.TypeFieldName = map[string]string{"DEMO": "Category"}
	issue := typedIssue("4", "")
	issue.CustomFields = []ytcli.CustomField{{Name: "Category", Value: map[string]interface{}{"name": "Initiative"}}}
	if got := issueTypeLabel(issue, m.cfg, ""); got != "Initiative" {
		t.Fatalf("custom type = %q", got)
	}
}

func TestTypeFilterCancelAndSave(t *testing.T) {
	m := typeFilterModel(t)
	m.openFilters()
	m.filterPanel.sections[2].Selected["Epic"] = true
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.selectedTypes) != 0 || m.filterMode {
		t.Fatal("cancel applied draft")
	}
	m.openFilters()
	m.filterPanel.sections[2].Selected = selection([]string{"Story", "Epic"})
	cmd := m.saveFilters()
	if cmd == nil || m.filterMode || !slices.Equal(m.selectedTypes, []string{"Epic", "Story"}) {
		t.Fatal("save did not start typed query")
	}
	saved, err := config.LoadConfig()
	if err != nil || len(saved.GetFilteredTypes("DEMO")) != 0 {
		t.Fatalf("session selection persisted: %v", err)
	}
	m.openFilters()
	clear(m.filterPanel.sections[2].Selected)
	m.saveFilters()
	if len(m.selectedTypes) != 0 || m.serverQuery != "" {
		t.Fatal("All types kept a query constraint")
	}
}

func TestInvalidTypeDoesNotApplyOtherDraftSelections(t *testing.T) {
	m := typeFilterModel(t)
	m.openFilters()
	clear(m.filterPanel.sections[0].Selected)
	m.filterPanel.sections[2].Selected = selection([]string{"bad{type"})
	if cmd := m.saveFilters(); cmd != nil || m.filterError == "" || !m.filterMode || len(m.excludedStates) > 0 || len(m.selectedTypes) > 0 {
		t.Fatal("invalid type partially applied the draft")
	}
}

func TestTypeFilterRejectsStaleResponsesAndSeparatesCaches(t *testing.T) {
	m := typeFilterModel(t)
	m.requestID = 3
	m.serverQuery = "Type: {Epic}"
	current := m.cacheKey()
	oldQuery := current
	oldQuery.query = "Type: {Story}"
	for _, msg := range []issuesDataMsg{
		{context: current, requestID: 1, limit: 20, issues: []ytcli.Issue{typedIssue("1", "Epic")}},
		{context: oldQuery, requestID: 3, limit: 20, issues: []ytcli.Issue{typedIssue("2", "Story")}},
		{context: current, requestID: 3, skip: 20, limit: 20, issues: []ytcli.Issue{typedIssue("3", "Epic")}},
	} {
		m, _ = m.Update(msg)
		if len(m.issues) > 0 || len(m.cache) > 0 {
			t.Fatal("stale response accepted")
		}
	}
	m, _ = m.Update(issuesDataMsg{context: current, requestID: 3, limit: 20, issues: []ytcli.Issue{typedIssue("4", "Epic")}})
	if len(m.issues) != 1 {
		t.Fatal("current response rejected")
	}
	m.cache[oldQuery] = projectCache{issues: []ytcli.Issue{typedIssue("2", "Story")}}
	m.invalidateCache("DEMO")
	if len(m.cache) != 0 {
		t.Fatal("project edit left filtered caches alive")
	}
}

func TestTypeMetadataFailureCannotSilentlyRemoveSavedFilter(t *testing.T) {
	m := typeFilterModel(t)
	m.selectedTypes = []string{"Epic"}
	m.typeInfoErr = errors.New("permission denied")
	if cmd := m.restoreOrLoadIssues(); cmd != nil || m.err == nil || m.loading {
		t.Fatal("metadata failure ignored saved filter")
	}
	m.openFilters()
	clear(m.filterPanel.sections[2].Selected)
	m.saveFilters()
	if m.err != nil || len(m.selectedTypes) > 0 {
		t.Fatal("All types did not recover from metadata failure")
	}
}

func TestProjectTypeFilteringBeforeLimitAndLocalFilters(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var issueQueries []string
	var offsets []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/admin/projects/0-1":
			fmt.Fprint(w, `{"id":"0-1","shortName":"DEMO"}`)
		case "/api/admin/projects/0-1/customFields":
			fmt.Fprint(w, `[{"field":{"name":"Type"},"$type":"EnumProjectCustomField","bundle":{"id":"1-2"}}]`)
		case "/api/admin/customFieldSettings/bundles/enum/1-2/values":
			fmt.Fprint(w, `[{"name":"Epic"},{"name":"Story"},{"name":"Task","archived":true}]`)
		case "/api/issues":
			query := r.URL.Query().Get("query")
			issueQueries = append(issueQueries, query)
			skip, _ := strconv.Atoi(r.URL.Query().Get("$skip"))
			offsets = append(offsets, skip)
			top, _ := strconv.Atoi(r.URL.Query().Get("$top"))
			// These issues occur after 500 tasks in the unfiltered project.
			all := []ytcli.Issue{typedIssue("501", "Epic"), typedIssue("502", "Story"), typedIssue("503", "Epic"), typedIssue("504", "Story")}
			if !strings.Contains(query, "Type: {Epic}, {Story}") {
				all = []ytcli.Issue{typedIssue("1", "Task")}
			}
			if skip >= len(all) {
				fmt.Fprint(w, "[]")
				return
			}
			json.NewEncoder(w).Encode(all[skip:min(len(all), skip+top)])
		default:
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))
	defer server.Close()
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.PageSize, cfg.MaxIssues = 2, 3
	client := ytcli.NewClient()
	client.SetCredentials(server.URL, "test-token")
	m := newIssuesModel(client, cfg)
	m.width, m.height = 110, 24
	cmd := m.initProject("0-1", false)
	m = runIssueCommands(t, m, cmd)
	offsets, issueQueries = nil, nil
	m.openFilters()
	m.filterPanel.sections[2].Selected = selection([]string{"Story", "Epic"})
	m = runIssueCommands(t, m, m.saveFilters())
	if m.err != nil || len(m.issues) != 3 || !m.limitReached {
		t.Fatalf("filtered load failed: issues=%d limit=%v error=%v", len(m.issues), m.limitReached, m.err)
	}
	if !slices.Equal(offsets, []int{0, 2}) || len(issueQueries) != 2 {
		t.Fatalf("pagination wrong: %v", offsets)
	}
	for _, q := range issueQueries {
		if q != "(project: 0-1) and (Type: {Epic}, {Story})" {
			t.Fatalf("query=%q", q)
		}
	}
	m.searchInput.SetValue("501")
	m.updateTableRows()
	if len(m.table.Rows()) != 1 || m.visibleIssueIDs[0] != "DEMO-501" {
		t.Fatal("local text filter failed with server filter")
	}
	m.excludedStates = []string{m.issues[0].State()}
	m.updateTableRows()
	if len(m.table.Rows()) != 0 {
		t.Fatal("state filter did not combine with type/text")
	}
	before := len(issueQueries)
	cmd = m.initProject("0-1", true)
	m = runIssueCommands(t, m, cmd)
	if len(issueQueries) != before || m.searchInput.Value() != "501" || !slices.Equal(m.selectedTypes, []string{"Epic", "Story"}) {
		t.Fatal("detail return lost context/cache")
	}
	view := stripAnsi(m.View())
	if !strings.Contains(view, "limit 3 reached") || !strings.Contains(view, "Types: Epic, Story") {
		t.Fatalf("missing filter/limit notice: %s", view)
	}
}

func TestTypeProjectAliasesShareSessionFilterAndRespectFieldConfiguration(t *testing.T) {
	for _, entry := range []string{"DEMO", "0-1"} {
		t.Run(entry, func(t *testing.T) {
			m := typeFilterModel(t)
			m.cfg.TypeFieldName = map[string]string{"0-1": "Category", "default": "Wrong field"}
			m.cfg.FilteredTypes = map[string][]string{"0-1": {"Initiative"}, "default": {"Bug"}}
			queries := make(chan string, 10)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/admin/projects":
					fmt.Fprint(w, `[{"id":"0-1","shortName":"DEMO"}]`)
				case "/api/admin/projects/0-1":
					fmt.Fprint(w, `{"id":"0-1","shortName":"DEMO"}`)
				case "/api/admin/projects/0-1/customFields":
					fmt.Fprint(w, `[{"field":{"name":"Category"},"bundle":{"id":"b1"}}]`)
				case "/api/admin/customFieldSettings/bundles/enum/b1/values":
					fmt.Fprint(w, `[{"name":"Initiative"}]`)
				case "/api/issues":
					queries <- r.URL.Query().Get("query")
					issue := typedIssue("1", "")
					issue.CustomFields = []ytcli.CustomField{{Name: "Category", Value: map[string]interface{}{"name": "Initiative"}}}
					json.NewEncoder(w).Encode([]ytcli.Issue{issue})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			m.client = ytcli.NewClient()
			m.client.SetCredentials(server.URL, "test-token")
			cmd := m.initProject(entry, false)
			m = runIssueCommands(t, m, cmd)
			if m.err != nil {
				t.Fatal(m.err)
			}
			if query := <-queries; strings.Contains(query, "Category:") {
				t.Fatalf("fresh session applied an old restriction: %q", query)
			}
			if got := m.table.Rows()[0][1]; got != "Initiative" {
				t.Fatalf("alias display = %q", got)
			}
			m.openFilters()
			m.filterPanel.sections[2].Selected = selection([]string{"Initiative"})
			m = runIssueCommands(t, m, m.saveFilters())
			if query := <-queries; !strings.Contains(query, "Category: {Initiative}") {
				t.Fatalf("missing selected type: %q", query)
			}
			other := "0-1"
			if entry == other {
				other = "DEMO"
			}
			cmd = m.initProject(other, false)
			m = runIssueCommands(t, m, cmd)
			if m.err != nil || !slices.Equal(m.selectedTypes, []string{"Initiative"}) {
				t.Fatalf("lost session filter on alias navigation: %v, %v", m.selectedTypes, m.err)
			}
		})
	}
}

func TestTypeAliasPrecedenceAndMetadataCacheMapping(t *testing.T) {
	m := typeFilterModel(t)
	m.cfg.TypeFieldName = map[string]string{"DEMO": "Category", "0-1": "Old Category", "default": "Type"}
	issue := typedIssue("1", "Task")
	issue.CustomFields = append(issue.CustomFields, ytcli.CustomField{Name: "Category", Value: map[string]interface{}{"name": "Initiative"}})
	for _, entry := range []string{"DEMO", "0-1", ""} {
		if got := issueTypeLabel(issue, m.cfg, entry); got != "Initiative" {
			t.Fatalf("%q label = %q", entry, got)
		}
	}
	key := m.typeKey()
	m.cfg.TypeFieldName["0-1"] = "Another Category"
	if key == m.typeKey() {
		t.Fatal("ID-key mapping change did not invalidate metadata cache key")
	}
}

func TestTypeMetadataRejectsOldContextGenerationAndServer(t *testing.T) {
	m := typeFilterModel(t)
	m.requestID = 4
	baseline := m.typeInfo
	stale := issueTypesMsg{key: m.typeKey(), requestID: 3, info: ytcli.IssueTypeInfo{FieldName: "Wrong"}}
	m, _ = m.Update(stale)
	if m.typeInfo.FieldName != baseline.FieldName {
		t.Fatal("old metadata generation applied")
	}
	stale.requestID = m.requestID
	stale.key.projectCode = "OTHER"
	m, _ = m.Update(stale)
	if m.typeInfo.FieldName != baseline.FieldName {
		t.Fatal("other project metadata applied")
	}
	m.client = ytcli.NewClient()
	m.client.SetCredentials("https://old.example", "test-token")
	m.serverURL = "https://old.example"
	stale.key = m.typeKey()
	m.client.SetCredentials("https://new.example", "test-token")
	m, _ = m.Update(stale)
	if m.typeInfo.FieldName != baseline.FieldName {
		t.Fatal("previous server metadata applied")
	}
}

func TestTypeFilterCombinesBaseORQueryExplicitly(t *testing.T) {
	m := typeFilterModel(t)
	m.selectedTypes = []string{"Epic"}
	m.query = "State: Open or Priority: Critical"
	m.restoreOrLoadIssues()
	if want := "(State: Open or Priority: Critical) and (Type: {Epic})"; m.serverQuery != want {
		t.Fatalf("query = %q, want %q", m.serverQuery, want)
	}
}

func TestUnavailableTypeMetadataDoesNotInventDefaultChoices(t *testing.T) {
	m := typeFilterModel(t)
	m.typeInfo, m.typeInfoErr = ytcli.IssueTypeInfo{}, errors.New("unavailable")
	m.selectedTypes = []string{"Saved kind"}
	m.issues = []ytcli.Issue{typedIssue("1", "Observed kind")}
	if got := m.availableTypes(); !slices.Equal(got, []string{"Observed kind", "Saved kind"}) {
		t.Fatalf("invented options: %v", got)
	}
}

func TestPendingMetadataUpdatesOnlyUntouchedTypeDraft(t *testing.T) {
	for _, edited := range []bool{false, true} {
		t.Run(fmt.Sprint(edited), func(t *testing.T) {
			m := typeFilterModel(t)
			resolved := m.typeInfo
			m.typeInfo = ytcli.IssueTypeInfo{}
			m.selectedTypes = []string{"epic"}
			m.openFilters()
			if edited {
				m.filterPanel.sections[2].Selected = selection([]string{"Story"})
			}
			m, _ = m.Update(issueTypesMsg{key: m.typeKey(), requestID: m.requestID, info: resolved})
			if !slices.Equal(m.selectedTypes, []string{"Epic"}) {
				t.Fatal("resolved saved filter was not applied")
			}
			want := "Epic"
			if edited {
				want = "Story"
			}
			if selected := selectedValues(m.filterPanel.sections[2].Options, m.filterPanel.sections[2].Selected); !slices.Equal(selected, []string{want}) {
				t.Fatalf("draft = %v, want %s", selected, want)
			}
		})
	}
}

func TestClearFiltersRejectsOldMetadataAndPage(t *testing.T) {
	m := tagFilterModel(t)
	m.selectedTypes, m.selectedTags = []string{"Epic"}, []string{"backend"}
	m.excludedStates, m.excludedPriorities = []string{"Open"}, []string{"Major"}
	m.searchInput.SetValue("needle")
	m.openFilters()
	oldType := issueTypesMsg{key: m.typeKey(), requestID: m.requestID, err: errors.New("old request")}
	oldPage := issuesDataMsg{context: m.cacheKey(), requestID: m.requestID, limit: 20, issues: []ytcli.Issue{typedIssue("1", "Epic")}}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m, _ = m.Update(oldType)
	m, _ = m.Update(oldPage)
	if m.filterMode || m.searchInput.Value() != "" || m.serverQuery != m.query || len(m.excludedStates)+len(m.excludedPriorities)+len(m.selectedTypes)+len(m.selectedTags) > 0 || len(m.issues) > 0 || m.typeInfoErr != nil {
		t.Fatal("clear failed or a stale response undid it")
	}
}

func TestTypeChoicesCanonicalizeSavedCaseWithoutDuplicates(t *testing.T) {
	m := typeFilterModel(t)
	m.selectedTypes = []string{"epic", "EPIC", "Archived Kind"}
	m.restoreOrLoadIssues()
	m.openFilters()
	if !slices.Equal(m.selectedTypes, []string{"Archived Kind", "Epic"}) {
		t.Fatalf("selection = %v", m.selectedTypes)
	}
	options := m.filterPanel.sections[2].Options
	if !slices.Equal(options, []string{"Archived Kind", "Epic", "Story", "Task"}) {
		t.Fatalf("options = %v", options)
	}
	if !m.filterPanel.sections[2].Selected["Epic"] {
		t.Fatal("canonical choice not checked")
	}
}

func TestUnresolvedProjectDoesNotActivateAnOldFilter(t *testing.T) {
	m := typeFilterModel(t)
	m.cfg.FilteredTypes = map[string][]string{"0-1": {"Epic"}}
	m.typeInfo = ytcli.IssueTypeInfo{}
	m.typeInfoErr = errors.New("project metadata unavailable")
	if cmd := m.restoreOrLoadIssues(); cmd == nil || m.err != nil || len(m.selectedTypes) != 0 {
		t.Fatal("legacy filtered_types prevented unfiltered browsing")
	}
}

func TestTypeFilterPanelFitsSmallTerminals(t *testing.T) {
	m := typeFilterModel(t)
	m.openFilters()
	for _, size := range [][2]int{{36, 12}, {80, 22}, {15, 6}} {
		m.width, m.height = size[0], size[1]
		view := m.View()
		if lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height {
			t.Fatalf("panel exceeds %v: %dx%d", size, lipgloss.Width(view), lipgloss.Height(view))
		}
	}
}

func TestNonProjectViewsDoNotInheritProjectTypeConstraint(t *testing.T) {
	m := typeFilterModel(t)
	m.cfg.SetFilteredTypes("default", []string{"Epic"})
	m.projectCode, m.query = "", "reporter: me"
	m.startContext(false)
	if len(m.selectedTypes) > 0 || m.serverQuery != "reporter: me" {
		t.Fatal("project type filter leaked into global view")
	}
	m.openFilters()
	if len(m.filterPanel.sections) != 2 {
		t.Fatal("global view offered incomplete type filtering")
	}
}
