package ytcli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListIssuesCombinesProjectAndQuery(t *testing.T) {
	// YouTrack requires explicit operators next to parenthesized expressions.
	// Keeping the entire query in one operand also binds every OR alternative
	// to the project, rather than allowing the final alternative to escape it.
	// https://www.jetbrains.com/help/youtrack/cloud/search-and-command-attributes.html#parentheses
	for _, tc := range []struct {
		name, project, query, want string
	}{
		{"all issues", "", "", ""},
		{"project only", "DEMO", "", "project: DEMO"},
		{"query only", "", "assignee: me #Unresolved", "assignee: me #Unresolved"},
		{"global OR", "", "Type: {Epic} or tag: {mind-v1}", "Type: {Epic} or tag: {mind-v1}"},
		{"type only", "DEMO", "Type: {Epic}", "(project: DEMO) and (Type: {Epic})"},
		{"tag only", "DEMO", "tag: {mind-v1}", "(project: DEMO) and (tag: {mind-v1})"},
		{"Epic and tag regression", "DEMO", "(Type: {Epic}) and (tag: {mind-v1})", "(project: DEMO) and ((Type: {Epic}) and (tag: {mind-v1}))"},
		{"multiple selections", "DEMO", "(Type: {Epic}, {Story}) and (tag: {mind-v1}, {needs review})", "(project: DEMO) and ((Type: {Epic}, {Story}) and (tag: {mind-v1}, {needs review}))"},
		{"project applies to every OR alternative", "DEMO", "Type: {Epic} or Type: {Story}", "(project: DEMO) and (Type: {Epic} or Type: {Story})"},
		{"nested OR", "DEMO", "((assignee: me or reporter: me) and (Type: {Epic})) and (tag: {mind-v1})", "(project: DEMO) and (((assignee: me or reporter: me) and (Type: {Epic})) and (tag: {mind-v1}))"},
		{"sort clause", "DEMO", "#Unresolved sort by: updated desc", "(project: DEMO) and (#Unresolved sort by: updated desc)"},
		{"literal punctuation", "DEMO", `summary: "release (v1)" tag: {R&D}`, `(project: DEMO) and (summary: "release (v1)" tag: {R&D})`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/issues" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				params := r.URL.Query()
				if got := params.Get("query"); got != tc.want {
					t.Errorf("query = %q; want %q", got, tc.want)
				}
				if params.Has("query") != (tc.want != "") {
					t.Errorf("query parameter presence = %v; want %v", params.Has("query"), tc.want != "")
				}
				if params.Get("$top") != "500" || params.Get("$skip") != "100" {
					t.Errorf("pagination changed: top=%q, skip=%q", params.Get("$top"), params.Get("$skip"))
				}
				fmt.Fprint(w, `[{"id":"2-1","idReadable":"DEMO-1"}]`)
			}))
			defer server.Close()
			client := &Client{baseURL: server.URL, token: "test-token", httpClient: server.Client()}
			issues, err := client.ListIssues(tc.project, tc.query, 500, 100)
			if err != nil || len(issues) != 1 || issues[0].IDReadable != "DEMO-1" {
				t.Fatalf("ListIssues returned %v, error=%v", issues, err)
			}
		})
	}
}
