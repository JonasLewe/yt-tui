package ytcli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestListProjectArticlesPaginatesScopedMetadata(t *testing.T) {
	var skips []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected method or authorization: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/youtrack/api/admin/projects":
			fmt.Fprint(w, `[{"id":"0-1","shortName":"OTHER"},{"id":"0-2","shortName":"DEMO"}]`)
		case "/youtrack/api/admin/projects/0-2/articles":
			query := r.URL.Query()
			if query.Get("$top") != "100" {
				t.Errorf("page size = %q, want 100", query.Get("$top"))
			}
			if query.Get("fields") != articleListFields {
				t.Errorf("unexpected metadata fields: %q", query.Get("fields"))
			}
			skip := query.Get("$skip")
			skips = append(skips, skip)
			// Deliberately return fewer than $top to exercise server page caps.
			switch skip {
			case "0":
				fmt.Fprint(w, `[{"id":"226-1","idReadable":"DEMO-A-1","summary":"Start","hasChildren":true,"ordinal":2,"project":{"id":"0-2","shortName":"DEMO"}}]`)
			case "1":
				fmt.Fprint(w, `[{"id":"226-2","idReadable":"DEMO-A-2","summary":"Child","parentArticle":{"id":"226-1"}}]`)
			case "2":
				fmt.Fprint(w, `[]`)
			default:
				t.Errorf("unexpected skip: %s", skip)
				http.Error(w, "bad pagination", http.StatusBadRequest)
			}
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &Client{baseURL: server.URL + "/youtrack/", token: "test-token", httpClient: server.Client()}
	articles, err := client.ListProjectArticles(" demo ")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(skips, []string{"0", "1", "2"}) {
		t.Fatalf("pagination skips = %v", skips)
	}
	if len(articles) != 2 || articles[0].IDReadable != "DEMO-A-1" || articles[0].Project.ShortName != "DEMO" {
		t.Fatalf("unexpected article list: %+v", articles)
	}
	if !articles[0].HasChildren || articles[0].Ordinal != 2 || articles[1].ParentArticle.ID != "226-1" {
		t.Fatalf("article hierarchy was not decoded: %+v", articles)
	}
}

func TestGetArticleReadsContentAndEscapesID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected method or authorization: %s", r.Method)
		}
		if got, want := r.URL.EscapedPath(), "/youtrack/api/articles/DEMO-A-1%2F%3F%23"; got != want {
			t.Errorf("escaped path = %q, want %q", got, want)
		}
		if r.URL.Query().Get("fields") != articleDetailFields {
			t.Errorf("unexpected detail fields: %s", r.URL.RawQuery)
		}
		fmt.Fprint(w, `{"id":"226-2","idReadable":"DEMO-A-2","summary":"Child","content":"# Guide\n\nArticle text","created":1000,"updated":2000,"parentArticle":{"id":"226-1","idReadable":"DEMO-A-1","summary":"Start"},"project":{"id":"0-2","shortName":"DEMO","name":"Demo Project"}}`)
	}))
	defer server.Close()
	client := &Client{baseURL: server.URL + "/youtrack", token: "test-token", httpClient: server.Client()}
	article, err := client.GetArticle("DEMO-A-1/?#")
	if err != nil {
		t.Fatal(err)
	}
	if article.Content != "# Guide\n\nArticle text" || article.Updated != 2000 || article.Created != 1000 || article.ParentArticle.Summary != "Start" {
		t.Fatalf("unexpected article: %+v", article)
	}
}

func TestArticleReadErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "forbidden", status: http.StatusForbidden, body: `{"error":"access_denied","error_description":"Read Article permission required"}`, want: "Read Article permission required"},
		{name: "missing", status: http.StatusNotFound, body: `{"error":"not_found","error_description":"Article not found"}`, want: "Article not found"},
		{name: "invalid JSON", status: http.StatusOK, body: `{`, want: "decode article"},
		{name: "null article", status: http.StatusOK, body: `null`, want: "no database ID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			client := &Client{baseURL: server.URL, token: "test-token", httpClient: server.Client()}
			if _, err := client.GetArticle("DEMO-A-1"); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestArticleNullContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":"226-1","content":null,"parentArticle":null}`)
	}))
	defer server.Close()
	client := &Client{baseURL: server.URL, token: "test-token", httpClient: server.Client()}
	article, err := client.GetArticle("DEMO-A-1")
	if err != nil || article == nil || article.Content != "" || article.ParentArticle != nil {
		t.Fatalf("null content: article=%+v, err=%v", article, err)
	}
}

func TestArticleRequestsValidateInputs(t *testing.T) {
	client := &Client{}
	if _, err := client.GetArticle("DEMO-A-1"); err == nil {
		t.Fatal("missing credentials should fail")
	}
	if _, err := client.ListProjectArticles("DEMO"); err == nil {
		t.Fatal("missing credentials should fail")
	}
	client.baseURL = "http://example.invalid"
	client.token = "test-token"
	for _, id := range []string{"", " ", ".", ".."} {
		if _, err := client.GetArticle(id); err == nil || !strings.Contains(err.Error(), "article ID") {
			t.Errorf("invalid article ID %q: %v", id, err)
		}
	}
	if _, err := client.ListProjectArticles(" "); err == nil || !strings.Contains(err.Error(), "project short name") {
		t.Fatalf("blank project: %v", err)
	}
}

func TestListProjectArticlesFailureDoesNotReturnPartialData(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "denied page", status: http.StatusForbidden, body: `{"error_description":"Article access denied"}`, want: "Article access denied"},
		{name: "malformed page", status: http.StatusOK, body: `{`, want: "decode project articles"},
		{name: "repeated page", status: http.StatusOK, body: `[{"id":"226-1"}]`, want: "duplicate article"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/api/admin/projects":
					fmt.Fprint(w, `[{"id":"0-1","shortName":"DEMO"}]`)
				case r.URL.Query().Get("$skip") == "0":
					fmt.Fprint(w, `[{"id":"226-1"}]`)
				default:
					w.WriteHeader(tc.status)
					fmt.Fprint(w, tc.body)
				}
			}))
			defer server.Close()
			client := &Client{baseURL: server.URL, token: "test-token", httpClient: server.Client()}
			articles, err := client.ListProjectArticles("DEMO")
			if err == nil || !strings.Contains(err.Error(), tc.want) || articles != nil {
				t.Fatalf("articles=%+v, error=%v; want no partial list and %q", articles, err, tc.want)
			}
		})
	}
}
