package ytcli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestIssueTypeValue(t *testing.T) {
	for _, tc := range []struct {
		name       string
		fields     string
		configured string
		want       string
	}{
		{"epic", `[{"name":"Type","value":{"name":"Epic"}}]`, "", "Epic"},
		{"story alias", `[{"name":"Issue Type","value":{"name":"Story"}}]`, "", "Story"},
		{"case insensitive", `[{"name":"tYpE","value":{"name":"Epic"}}]`, "", "Epic"},
		{"custom field", `[{"name":"Work Category","value":{"name":"Story"}}]`, "work category", "Story"},
		{"type preference", `[{"name":"Issue Type","value":{"name":"Story"}},{"name":"Type","value":{"name":"Epic"}}]`, "", "Epic"},
		{"configured only", `[{"name":"Type","value":{"name":"Epic"}}]`, "Work Category", ""},
		{"missing", `[]`, "", ""},
		{"nil value", `[{"name":"Type","value":null}]`, "", ""},
		{"empty value", `[{"name":"Type","value":{"name":""}}]`, "", ""},
		{"empty object", `[{"name":"Type","value":{}}]`, "", ""},
		{"unavailable name", `[{"name":"Type","value":{"id":"11-1"}}]`, "", ""},
		{"not an enum", `[{"name":"Type","value":true}]`, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var issue Issue
			if err := json.Unmarshal([]byte(`{"customFields":`+tc.fields+`}`), &issue); err != nil {
				t.Fatal(err)
			}
			if got := issue.TypeValue(tc.configured); got != tc.want {
				t.Fatalf("TypeValue(%q) = %q, want %q", tc.configured, got, tc.want)
			}
			if tc.configured == "" && issue.Type() != tc.want {
				t.Fatalf("Type() = %q, want %q", issue.Type(), tc.want)
			}
		})
	}
}

func TestIssueTypeQuery(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		values      []string
		want        string
		wantError   bool
	}{
		{"all", "", nil, "", false},
		{"single", "Type", []string{"Epic"}, "Type: {Epic}", false},
		{"OR", "Type", []string{"Epic", "Story"}, "Type: {Epic}, {Story}", false},
		{"spaces", "Work Category", []string{"User Story"}, "Work Category: {User Story}", false},
		{"unicode", "Vorgangstyp", []string{"Änderung"}, "Vorgangstyp: {Änderung}", false},
		{"literal punctuation", "Type", []string{"Story, Task: (draft)", "Won't fix"}, "Type: {Story, Task: (draft)}, {Won't fix}", false},
		{"deduplicate", "Type", []string{"Epic", "epic", "Story"}, "Type: {Epic}, {Story}", false},
		{"missing field", "", []string{"Story"}, "", true},
		{"empty value", "Type", []string{""}, "", true},
		{"brace injection", "Type", []string{"Epic} or project: Other {"}, "", true},
		{"backslash", "Type", []string{`Epic\}`}, "", true},
		{"control", "Type", []string{"Epic\nStory"}, "", true},
		{"wildcard", "Type", []string{"Epic*"}, "", true},
		{"field injection", "Type: Epic or project", []string{"Story"}, "", true},
		{"field operator", "Type or Category", []string{"Story"}, "", true},
		{"field newline", "Type\nCategory", []string{"Story"}, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := IssueTypeQuery(tc.field, tc.values)
			if (err != nil) != tc.wantError || got != tc.want {
				t.Fatalf("IssueTypeQuery = %q, %v; want %q, error=%v", got, err, tc.want, tc.wantError)
			}
		})
	}
}

func TestGetProjectIssueTypes(t *testing.T) {
	for _, tc := range []struct {
		name, configured, fields string
		wantField                string
		wantFound, wantError     bool
	}{
		{"Type", "", `[{"field":{"name":"Type"},"bundle":{"id":"b1"},"$type":"EnumProjectCustomField"}]`, "Type", true, false},
		{"Issue Type", "", `[{"field":{"name":"Issue Type"},"bundle":{"id":"b1"}}]`, "Issue Type", true, false},
		{"custom", "work category", `[{"field":{"name":"Work Category"},"bundle":{"id":"b1"}}]`, "Work Category", true, false},
		{"prefer Type", "", `[{"field":{"name":"Issue Type"},"bundle":{"id":"wrong"}},{"field":{"name":"TYPE"},"bundle":{"id":"b1"}}]`, "TYPE", true, false},
		{"no field", "", `[{"field":{"name":"Priority"},"bundle":{"id":"b1"}}]`, "", false, false},
		{"custom missing", "Work Category", `[{"field":{"name":"Type"},"bundle":{"id":"b1"}}]`, "", false, false},
		{"no bundle", "", `[{"field":{"name":"Type"}}]`, "Type", true, true},
		{"not enum", "", `[{"field":{"name":"Type"},"bundle":{"id":"b1"},"$type":"StateProjectCustomField"}]`, "Type", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("missing authorization")
				}
				switch r.URL.Path {
				case "/api/admin/projects/0-1/customFields":
					fmt.Fprint(w, tc.fields)
				case "/api/admin/customFieldSettings/bundles/enum/b1/values":
					fmt.Fprint(w, `[{"name":"Story","ordinal":2},{"name":"Old Epic","ordinal":0,"archived":true},{"name":"Epic","ordinal":1},{"name":"","ordinal":0},{"name":"Epic","ordinal":1}]`)
				default:
					t.Errorf("unexpected request %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			client := &Client{baseURL: server.URL, token: "test-token", httpClient: server.Client()}
			info, err := client.GetProjectIssueTypes("0-1", tc.configured)
			if (err != nil) != tc.wantError || info.FieldName != tc.wantField || info.Found != tc.wantFound {
				t.Fatalf("GetProjectIssueTypes = %#v, %v", info, err)
			}
			if tc.wantFound && !tc.wantError && !reflect.DeepEqual(info.Values, []string{"Old Epic", "Epic", "Story"}) {
				t.Fatalf("options = %v; archived values must remain searchable and ordering must follow ordinal", info.Values)
			}
		})
	}
}

func TestGetProjectIssueTypesPaginationAndHyphenatedProject(t *testing.T) {
	var projectSkips, metadataSkips, bundleSkips []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch r.URL.Path {
		case "/api/admin/projects":
			projectSkips = append(projectSkips, query.Get("$skip"))
			if query.Get("$skip") == "0" {
				projects := make([]Project, 200)
				for i := range projects {
					projects[i] = Project{ID: fmt.Sprintf("0-%d", i+2), ShortName: fmt.Sprintf("PROJ%d", i)}
				}
				json.NewEncoder(w).Encode(projects)
			} else {
				fmt.Fprint(w, `[{"id":"0-1","shortName":"MY-PROJECT"}]`)
			}
		case "/api/admin/projects/0-1/customFields":
			metadataSkips = append(metadataSkips, query.Get("$skip"))
			if query.Get("$top") != "200" {
				t.Errorf("metadata page size = %q", query.Get("$top"))
			}
			if query.Get("$skip") == "0" {
				fields := make([]map[string]any, 200)
				for i := range fields {
					fields[i] = map[string]any{"field": map[string]string{"name": fmt.Sprintf("Field%d", i)}}
				}
				json.NewEncoder(w).Encode(fields)
			} else {
				fmt.Fprint(w, `[{"field":{"name":"Type"},"bundle":{"id":"b1"}}]`)
			}
		case "/api/admin/customFieldSettings/bundles/enum/b1/values":
			bundleSkips = append(bundleSkips, query.Get("$skip"))
			skip, _ := strconv.Atoi(query.Get("$skip"))
			count := 200
			if skip == 200 {
				count = 1
			}
			values := make([]issueTypeBundleValue, count)
			for i := range values {
				values[i] = issueTypeBundleValue{Name: fmt.Sprintf("Type %d", skip+i), Ordinal: skip + i}
			}
			json.NewEncoder(w).Encode(values)
		default:
			t.Errorf("unexpected request %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &Client{baseURL: server.URL, token: "test-token", httpClient: server.Client()}
	info, err := client.GetProjectIssueTypes("MY-PROJECT", "")
	if err != nil || !info.Found || len(info.Values) != 201 || info.Values[200] != "Type 200" {
		t.Fatalf("pagination result = %#v, %v", info, err)
	}
	if !reflect.DeepEqual(projectSkips, []string{"0", "200"}) || !reflect.DeepEqual(metadataSkips, []string{"0", "200"}) || !reflect.DeepEqual(bundleSkips, []string{"0", "200"}) {
		t.Fatalf("project skips %v, metadata skips %v, bundle skips %v", projectSkips, metadataSkips, bundleSkips)
	}
}

func TestGetProjectIssueTypesReportsErrors(t *testing.T) {
	for _, failingEndpoint := range []string{"customFields", "values"} {
		for _, response := range []struct {
			name, body string
			status     int
		}{
			{"forbidden", `{"error":"Forbidden","error_description":"Read access denied"}`, http.StatusForbidden},
			{"malformed", `{`, http.StatusOK},
		} {
			t.Run(failingEndpoint+"/"+response.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.HasSuffix(r.URL.Path, "/"+failingEndpoint) {
						w.WriteHeader(response.status)
						fmt.Fprint(w, response.body)
						return
					}
					fmt.Fprint(w, `[{"field":{"name":"Type"},"bundle":{"id":"b1"}}]`)
				}))
				defer server.Close()
				client := &Client{baseURL: server.URL, token: "test-token", httpClient: server.Client()}
				if _, err := client.GetProjectIssueTypes("0-1", ""); err == nil {
					t.Fatal("metadata failures must not be reported as no type field")
				}
			})
		}
	}
}
