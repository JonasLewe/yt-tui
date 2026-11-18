package ytcli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestTagQuery(t *testing.T) {
	for _, tc := range []struct {
		name string
		tags []string
		want string
		err  bool
	}{
		{"all", nil, "", false},
		{"single", []string{"backend"}, "tag: {backend}", false},
		{"OR and spaces", []string{"backend", "needs review"}, "tag: {backend}, {needs review}", false},
		{"unicode", []string{"Änderung 🚀"}, "tag: {Änderung 🚀}", false},
		{"punctuation", []string{`UI, API: (v2)`, `Won't fix`, `"quoted"`, `-negation`}, `tag: {UI, API: (v2)}, {Won't fix}, {"quoted"}, {-negation}`, false},
		{"duplicate name", []string{"backend", "Backend", "frontend"}, "tag: {backend}, {frontend}", false},
		{"empty", []string{""}, "", true},
		{"blank", []string{"  "}, "", true},
		{"brace injection", []string{"backend} or project: Other {"}, "", true},
		{"opening brace", []string{"{backend"}, "", true},
		{"backslash", []string{`back\end`}, "", true},
		{"wildcard", []string{"backend*"}, "", true},
		{"question mark", []string{"backend?"}, "", true},
		{"newline", []string{"backend\nfrontend"}, "", true},
		{"ANSI control", []string{"\x1b[31mbackend"}, "", true},
		{"partial invalid", []string{"backend", "bad}"}, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := TagQuery(tc.tags)
			if got != tc.want || (err != nil) != tc.err {
				t.Fatalf("TagQuery = %q, %v; want %q, error=%v", got, err, tc.want, tc.err)
			}
		})
	}
}

func TestListTagsPaginatesCompleteVisibleSet(t *testing.T) {
	var skips []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/youtrack/api/tags" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing authorization")
		}
		query := r.URL.Query()
		if query.Get("fields") != "id,name,owner(id,login)" || query.Get("$top") != "200" {
			t.Errorf("unexpected fields/pagination: %v", query)
		}
		skip, _ := strconv.Atoi(query.Get("$skip"))
		skips = append(skips, skip)
		// An installation caps responses at 42, even when $top asks for 200.
		var page []Tag
		for i := skip; i < min(skip+42, 517); i++ {
			page = append(page, Tag{ID: fmt.Sprintf("6-%d", i), Name: "same visible name", Owner: &User{ID: fmt.Sprintf("1-%d", i), Login: fmt.Sprintf("user%d", i)}})
		}
		json.NewEncoder(w).Encode(page)
	}))
	defer server.Close()
	client := &Client{baseURL: server.URL + "/youtrack/", token: "test-token", httpClient: server.Client()}
	tags, err := client.ListTags()
	if err != nil || len(tags) != 517 {
		t.Fatalf("ListTags returned %d tags, error=%v", len(tags), err)
	}
	if len(skips) != 14 || skips[13] != 517 || tags[516].ID != "6-516" || tags[516].Owner.ID != "1-516" {
		t.Fatalf("incomplete pagination or identity: skips=%v, final=%+v", skips, tags[len(tags)-1])
	}
}

func TestListTagsErrorsDiscardPartialResults(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"API permission", http.StatusForbidden, `{"error":"forbidden","error_description":"Access denied"}`},
		{"malformed page", http.StatusOK, `[{`},
		{"missing name", http.StatusOK, `[{"id":"6-2"}]`},
		{"missing ID", http.StatusOK, `[{"name":"backend"}]`},
		{"ignored skip", http.StatusOK, `[{"id":"6-1","name":"backend"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("$skip") == "0" {
					fmt.Fprint(w, `[{"id":"6-1","name":"backend"}]`)
					return
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			client := &Client{baseURL: server.URL, token: "test-token", httpClient: server.Client()}
			if tags, err := client.ListTags(); err == nil || tags != nil {
				t.Fatalf("expected failed complete set, got %v, %v", tags, err)
			}
		})
	}
	for _, client := range []*Client{nil, {}, {baseURL: "https://example.test"}, {token: "test-token"}} {
		if _, err := client.ListTags(); err == nil {
			t.Fatal("missing credentials accepted")
		}
	}
}

func TestTagFilterScopeSeparatesAccountsAndPreservesTokenRotation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/youtrack/api/users/me" || r.URL.Query().Get("fields") != "id,login" {
			t.Errorf("unexpected request: %s", r.URL)
		}
		id := "1-1"
		if r.Header.Get("Authorization") == "Bearer another-account" {
			id = "1-2"
		}
		json.NewEncoder(w).Encode(User{ID: id, Login: "test-user"})
	}))
	defer server.Close()
	client := &Client{baseURL: server.URL + "/youtrack/", token: "first-token", httpClient: server.Client()}
	firstScope, err := client.TagFilterScope()
	if err != nil || firstScope != server.URL+"/youtrack#1-1" {
		t.Fatalf("scope=%q, err=%v", firstScope, err)
	}
	firstContext := client.ContextKey()
	client.token = "rotated-token"
	rotatedScope, err := client.TagFilterScope()
	if err != nil || rotatedScope != firstScope || client.ContextKey() == firstContext {
		t.Fatal("rotating tokens must keep saved filters and invalidate in-memory context")
	}
	client.token = "another-account"
	otherScope, err := client.TagFilterScope()
	if err != nil || otherScope == firstScope {
		t.Fatal("different accounts must have different filter scopes")
	}
	for _, secret := range []string{"first-token", "rotated-token", "another-account"} {
		if strings.Contains(firstContext, secret) || strings.Contains(firstScope, secret) {
			t.Fatal("scope must not expose credentials")
		}
	}
	client.baseURL = "https://another-server.test/youtrack"
	if firstContext == client.ContextKey() {
		t.Fatal("servers must not share contexts")
	}
}

func TestTagFilterScopeErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"missing identity", http.StatusOK, `{"login":"someone"}`},
		{"malformed", http.StatusOK, `{`},
		{"denied", http.StatusForbidden, `{"error":"forbidden"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			client := &Client{baseURL: server.URL, token: "test-token", httpClient: server.Client()}
			if scope, err := client.TagFilterScope(); err == nil || scope != "" {
				t.Fatalf("failed identity returned scope=%q, err=%v", scope, err)
			}
		})
	}
}

func TestIssuesRequestAndDecodeNativeTags(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if !strings.Contains(r.URL.Query().Get("fields"), "tags(id,name,owner(id,login))") {
			t.Errorf("%s did not request native tags", r.URL.Path)
		}
		issue := `{"id":"2-1","idReadable":"APP-1","tags":[{"id":"6-1","name":"needs review","owner":{"id":"1-1","login":"jonas"}}]}`
		switch r.URL.Path {
		case "/api/issues":
			fmt.Fprint(w, "["+issue+"]")
		case "/api/issues/APP-1":
			fmt.Fprint(w, issue)
		case "/api/agiles/1/sprints/2":
			fmt.Fprint(w, `{"issues":[`+issue+`]}`)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client := &Client{baseURL: server.URL, token: "test-token", httpClient: server.Client()}
	list, err := client.ListIssues("APP", "tag: {needs review}", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := client.GetIssue("APP-1")
	if err != nil {
		t.Fatal(err)
	}
	sprint, err := client.ListSprintIssues("1", "2")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || len(sprint) != 1 || detail == nil {
		t.Fatalf("missing issue payload: list=%v, detail=%v, sprint=%v", list, detail, sprint)
	}
	for _, issue := range []Issue{list[0], *detail, sprint[0]} {
		if len(issue.Tags) != 1 || issue.Tags[0].Name != "needs review" || issue.Tags[0].Owner == nil || issue.Tags[0].Owner.ID != "1-1" {
			t.Fatalf("tags missing from issue: %+v", issue)
		}
	}
	if !slices.Equal(paths, []string{"/api/issues", "/api/issues/APP-1", "/api/agiles/1/sprints/2"}) {
		t.Fatalf("unexpected request paths: %v", paths)
	}
}
