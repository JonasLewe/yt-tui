package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"yt-tui/internal/config"
	"yt-tui/internal/ytcli"
)

func TestCombinedTypeTagFilterHTTPQuery(t *testing.T) {
	// YouTrack requires an explicit operator before a parenthesized expression.
	// Check the complete request, including the project restriction added by the
	// API client, rather than accepting independently valid filter fragments.
	// https://www.jetbrains.com/help/youtrack/cloud/search-and-command-attributes.html#parentheses
	for _, tc := range []struct {
		name, baseQuery string
		types, tags     []string
		wantQuery       string
	}{
		{
			name:      "reported Epic and mind-v1 combination",
			types:     []string{"Epic"},
			tags:      []string{"mind-v1"},
			wantQuery: "(project: DEMO) and ((Type: {Epic}) and (tag: {mind-v1}))",
		},
		{
			name:      "multiple types and tags retain OR within each filter",
			types:     []string{"Story", "Epic"},
			tags:      []string{"needs review", "mind-v1"},
			wantQuery: "(project: DEMO) and ((Type: {Epic}, {Story}) and (tag: {mind-v1}, {needs review}))",
		},
		{
			name:      "base OR remains restricted by project type and tag",
			baseQuery: "State: Open or Priority: Critical",
			types:     []string{"Epic"},
			tags:      []string{"mind-v1"},
			wantQuery: "(project: DEMO) and (((State: Open or Priority: Critical) and (Type: {Epic})) and (tag: {mind-v1}))",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			queries := make(chan string, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/issues" {
					http.Error(w, "unexpected request", http.StatusNotFound)
					return
				}
				query := r.URL.Query().Get("query")
				queries <- query
				w.Header().Set("Content-Type", "application/json")
				if query != tc.wantQuery {
					w.WriteHeader(http.StatusBadRequest)
					fmt.Fprint(w, `{"error":"invalid_query","error_description":"unexpected combined search query"}`)
					return
				}
				json.NewEncoder(w).Encode([]ytcli.Issue{typedIssue("1", "Epic")})
			}))
			defer server.Close()

			m := tagFilterModel(t)
			client := ytcli.NewClient()
			client.SetCredentials(server.URL, "test-token")
			m.client, m.serverURL, m.contextKey = client, server.URL, client.ContextKey()
			m.tagScope = server.URL + "#user-1"
			m.tags = []ytcli.Tag{{ID: "1", Name: "mind-v1"}, {ID: "2", Name: "needs review"}}
			m.query = tc.baseQuery
			m.typeInfoCache[m.typeKey()] = m.typeInfo
			m.openFilters()
			m.filterPanel.sections[2].Selected = selection(tc.types)
			m.filterPanel.sections[3].Selected = selection(tc.tags)
			m = runIssueCommands(t, m, m.saveFilters())
			assertCombinedQueryResult(t, m, queries, tc.wantQuery)

			// Restoring the saved filter must use the same complete server query.
			saved, err := config.LoadConfig()
			if err != nil {
				t.Fatal(err)
			}
			fresh := newIssuesModel(client, saved)
			fresh.projectCode, fresh.query = "DEMO", tc.baseQuery
			fresh.serverURL, fresh.contextKey = server.URL, client.ContextKey()
			fresh.tagScope, fresh.tagScopeReady = m.tagScope, true
			fresh.typeInfoCache[fresh.typeKey()] = m.typeInfo
			fresh = runIssueCommands(t, fresh, fresh.startContext(false))
			assertCombinedQueryResult(t, fresh, queries, tc.wantQuery)
		})
	}
}

func assertCombinedQueryResult(t *testing.T, m issuesModel, queries <-chan string, wantQuery string) {
	t.Helper()
	select {
	case got := <-queries:
		if got != wantQuery {
			t.Fatalf("complete issue query = %q, want %q", got, wantQuery)
		}
	default:
		t.Fatal("filter did not send an issue request")
	}
	if m.err != nil || m.loading || len(m.issues) != 1 || m.issues[0].IDReadable != "DEMO-1" {
		t.Fatalf("combined filter did not load issue: err=%v loading=%v issues=%v", m.err, m.loading, m.issues)
	}
}
