package ytcli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestCreateArticlePreservesContentAndOptionalParent(t *testing.T) {
	for _, parentID := range []string{"", "226-7"} {
		t.Run("parent="+parentID, func(t *testing.T) {
			content := "# Guide\n\n  Indented content.\n\n"
			var writes int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("missing authentication")
				}
				switch r.URL.Path {
				case "/youtrack/api/admin/projects":
					if r.Method != http.MethodGet {
						t.Errorf("project lookup method = %s", r.Method)
					}
					fmt.Fprint(w, `[{"id":"0-1","shortName":"DEMO"}]`)
				case "/youtrack/api/articles":
					writes++
					if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
						t.Errorf("unexpected create request: %s, %s", r.Method, r.Header.Get("Content-Type"))
					}
					if r.URL.Query().Get("fields") != "id,idReadable" {
						t.Errorf("unexpected response fields: %s", r.URL.RawQuery)
					}
					var got map[string]interface{}
					if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
						t.Error(err)
					}
					want := map[string]interface{}{
						"project": map[string]interface{}{"id": "0-1"},
						"summary": "Article title", "content": content,
					}
					if parentID != "" {
						want["parentArticle"] = map[string]interface{}{"id": parentID}
					}
					if !reflect.DeepEqual(got, want) {
						t.Errorf("create payload = %#v, want %#v", got, want)
					}
					w.WriteHeader(http.StatusCreated)
					fmt.Fprint(w, `{"id":"226-8","idReadable":"DEMO-A-8"}`)
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			client := &Client{baseURL: server.URL + "/youtrack/", token: "test-token", httpClient: server.Client()}
			id, err := client.CreateArticle(" demo ", "Article title", content, parentID)
			if err != nil || id != "DEMO-A-8" || writes != 1 {
				t.Fatalf("create: id=%q error=%v writes=%d", id, err, writes)
			}
		})
	}
}

func TestUpdateArticleWritesOnlyTitleAndContent(t *testing.T) {
	for _, content := range []string{"# Title\n\nText\n", ""} {
		t.Run("content="+content, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-token" {
					t.Errorf("unexpected update request: %s", r.Method)
				}
				if r.URL.EscapedPath() != "/youtrack/api/articles/DEMO-A-1%2F%3F%23" {
					t.Errorf("article ID not safely encoded: %s", r.URL.EscapedPath())
				}
				var got map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Error(err)
				}
				want := map[string]interface{}{"summary": "New title", "content": content}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("update payload = %#v, want %#v", got, want)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			client := &Client{baseURL: server.URL + "/youtrack/", token: "test-token", httpClient: server.Client()}
			if err := client.UpdateArticle("DEMO-A-1/?#", "New title", content); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestArticleWriteValidationMakesNoRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("invalid input sent request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	client := &Client{baseURL: server.URL, token: "test-token", httpClient: server.Client()}
	for _, args := range [][4]string{{"", "Title", "", ""}, {"DEMO", " \n", "", ""}, {"DEMO", "Title", "", " "}, {"DEMO", "Title", "", "."}, {"DEMO", "Title", "", ".."}} {
		if _, err := client.CreateArticle(args[0], args[1], args[2], args[3]); err == nil {
			t.Errorf("invalid create accepted: %q", args)
		}
	}
	for _, id := range []string{"", " ", ".", ".."} {
		if err := client.UpdateArticle(id, "Title", ""); err == nil {
			t.Errorf("invalid update ID accepted: %q", id)
		}
	}
	if err := client.UpdateArticle("DEMO-A-1", " \n", ""); err == nil {
		t.Error("blank update title accepted")
	}
	client.token = ""
	if _, err := client.CreateArticle("DEMO", "Title", "", ""); err == nil {
		t.Error("create without credentials accepted")
	}
	if err := client.UpdateArticle("DEMO-A-1", "Title", ""); err == nil {
		t.Error("update without credentials accepted")
	}
}

func TestArticleWriteResponses(t *testing.T) {
	for _, tc := range []struct {
		name      string
		create    bool
		status    int
		body      string
		wantID    string
		wantError string
	}{
		{name: "create database ID fallback", create: true, status: http.StatusOK, body: `{"id":"226-1"}`, wantID: "226-1"},
		{name: "create empty body", create: true, status: http.StatusCreated, wantError: "decode created article"},
		{name: "create missing ID", create: true, status: http.StatusCreated, body: `{}`, wantError: "no ID"},
		{name: "create permission denied", create: true, status: http.StatusForbidden, body: `{"error_description":"Create Article permission required"}`, wantError: "Create Article permission required"},
		{name: "update empty success body", status: http.StatusOK},
		{name: "update permission denied", status: http.StatusForbidden, body: `{"error_description":"Update Article permission required"}`, wantError: "Update Article permission required"},
		{name: "update missing article", status: http.StatusNotFound, wantError: "404"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/admin/projects" {
					fmt.Fprint(w, `[{"id":"0-1","shortName":"DEMO"}]`)
					return
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			client := &Client{baseURL: server.URL, token: "test-token", httpClient: server.Client()}
			var id string
			var err error
			if tc.create {
				id, err = client.CreateArticle("DEMO", "Title", "", "")
			} else {
				err = client.UpdateArticle("DEMO-A-1", "Title", "")
			}
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v, want %q", err, tc.wantError)
				}
			} else if err != nil || id != tc.wantID {
				t.Fatalf("id=%q error=%v, want ID %q and no error", id, err, tc.wantID)
			}
		})
	}
}
