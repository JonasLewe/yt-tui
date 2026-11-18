package ytcli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestIssueCreateProjectMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/admin/projects/0-1/customFields":
			_, _ = w.Write([]byte(`[
				{"$type":"UserProjectCustomField","field":{"name":"Reviewer"},"canBeEmpty":true},
				{"$type":"UserProjectCustomField","field":{"name":"Assignee","fieldType":{"isMultiValue":true}},"canBeEmpty":false},
				{"$type":"StateProjectCustomField","field":{"name":"Stage"},"bundle":{"id":"b1"}}
			]`))
		case "/api/admin/customFieldSettings/bundles/state/b1/values":
			_, _ = w.Write([]byte(`[{"id":"s1","name":"Ready"},{"id":"s2","name":"Removed","archived":true}]`))
		default:
			t.Errorf("unexpected metadata path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := &Client{baseURL: server.URL, token: "t", httpClient: server.Client()}
	meta, err := c.GetIssueCreateFields("0-1")
	if err != nil || !meta.AssigneeMulti || !meta.AssigneeRequired || meta.AssigneeName != "Assignee" || meta.StateName != "Stage" || !reflect.DeepEqual(meta.States, []CreateState{{ID: "s1", Name: "Ready"}}) {
		t.Fatalf("unexpected metadata %+v, %v", meta, err)
	}
}

func TestIssueCreationDefaultsAndSelections(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		opts                 CreateIssueOptions
		assignee             string
		wantFields, wantTags string
		wantError            bool
	}{
		{"unassigned with server default state", CreateIssueOptions{Fields: &IssueCreateFields{AssigneeName: "Assignee", StateName: "Stage"}}, "", `[{"name":"Assignee","$type":"SingleUserIssueCustomField","value":null}]`, "", false},
		{"explicit state and same-name tags by id", CreateIssueOptions{Fields: &IssueCreateFields{StateName: "Stage"}, StateID: "s1", TagIDs: []string{"6-1", "6-2"}}, "", `[{"name":"Stage","$type":"StateIssueCustomField","value":{"id":"s1"}}]`, `[{"id":"6-1"},{"id":"6-2"}]`, false},
		{"project without assignee", CreateIssueOptions{Fields: &IssueCreateFields{}}, "", "", "", false},
		{"optional multi assignee", CreateIssueOptions{Fields: &IssueCreateFields{AssigneeName: "Assignee", AssigneeMulti: true}}, "unassigned", `[{"name":"Assignee","$type":"MultiUserIssueCustomField","value":[]}]`, "", false},
		{"required assignee", CreateIssueOptions{Fields: &IssueCreateFields{AssigneeName: "Assignee", AssigneeRequired: true}}, "", "", "", true},
		{"user explicitly chosen", CreateIssueOptions{Fields: &IssueCreateFields{AssigneeName: "Assignee", AssigneeRequired: true}}, "Jane.X", `[{"name":"Assignee","$type":"SingleUserIssueCustomField","value":{"login":"Jane.X"}}]`, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			posts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/admin/projects":
					_, _ = w.Write([]byte(`[{"id":"0-1","shortName":"DEMO"}]`))
				case "/api/issues":
					posts++
					if r.Method != http.MethodPost {
						t.Errorf("method %s", r.Method)
					}
					var got map[string]json.RawMessage
					if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
						t.Error(err)
					}
					for key, want := range map[string]string{"customFields": tc.wantFields, "tags": tc.wantTags} {
						if want == "" {
							if _, ok := got[key]; ok {
								t.Errorf("unexpected %s: %s", key, got[key])
							}
							continue
						}
						var actual, expected any
						if err := json.Unmarshal(got[key], &actual); err != nil {
							t.Error(err)
							continue
						}
						if err := json.Unmarshal([]byte(want), &expected); err != nil {
							t.Error(err)
							continue
						}
						if !reflect.DeepEqual(actual, expected) {
							t.Errorf("%s = %s, want %s", key, got[key], want)
						}
					}
					_, _ = w.Write([]byte(`{"id":"2-1","idReadable":"DEMO-1"}`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			c := &Client{baseURL: server.URL, token: "t", httpClient: server.Client()}
			id, err := c.CreateIssueWithOptions("DEMO", "Feature", "", "", "", tc.assignee, tc.opts)
			if tc.wantError {
				if err == nil || posts != 0 {
					t.Fatalf("expected validation error before create, got %v, %d posts", err, posts)
				}
			} else if err != nil || id != "DEMO-1" || posts != 1 {
				t.Fatalf("create = %q, %v, %d posts", id, err, posts)
			}
		})
	}
}
