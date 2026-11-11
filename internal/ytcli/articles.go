package ytcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Article represents a published YouTrack Knowledge Base article.
type Article struct {
	ID            string   `json:"id"`
	IDReadable    string   `json:"idReadable"`
	Summary       string   `json:"summary"`
	Content       string   `json:"content"`
	Created       int64    `json:"created"`
	Updated       int64    `json:"updated"`
	Project       *Project `json:"project"`
	ParentArticle *Article `json:"parentArticle"`
	HasChildren   bool     `json:"hasChildren"`
	Ordinal       int64    `json:"ordinal"`
}

const articleListFields = "id,idReadable,summary,hasChildren,ordinal,parentArticle(id),project(id,shortName,name)"
const articleDetailFields = "id,idReadable,summary,content,created,updated,hasChildren,ordinal,parentArticle(id,idReadable,summary),project(id,shortName,name)"

// ListProjectArticles loads all visible article summaries for a project, including
// nested articles. Content is fetched separately when an article is opened.
func (c *Client) ListProjectArticles(projectCode string) ([]Article, error) {
	if c.baseURL == "" || c.token == "" {
		return nil, errors.New("missing YouTrack connection URL or token")
	}
	projectCode = strings.TrimSpace(projectCode)
	if projectCode == "" {
		return nil, errors.New("project short name is required")
	}
	projectID, err := c.getProjectIDByShortName(projectCode)
	if err != nil {
		return nil, err
	}
	if !validArticlePathID(projectID) {
		return nil, errors.New("project has no valid database ID")
	}

	endpoint := strings.TrimRight(c.baseURL, "/") + "/api/admin/projects/" + url.PathEscape(projectID) + "/articles"
	query := url.Values{
		"fields": {articleListFields},
		"$top":   {"100"},
	}
	articles := make([]Article, 0)
	seen := make(map[string]bool)
	for skip := 0; ; {
		query.Set("$skip", strconv.Itoa(skip))
		body, err := c.readArticleResource(endpoint + "?" + query.Encode())
		if err != nil {
			return nil, err
		}
		var page []Article
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("decode project articles: %w", err)
		}
		if len(page) == 0 {
			return articles, nil
		}
		for _, article := range page {
			if article.ID == "" {
				return nil, errors.New("project article has no database ID")
			}
			if seen[article.ID] {
				return nil, errors.New("article pagination returned a duplicate article; refresh the Knowledge Base")
			}
			seen[article.ID] = true
		}
		articles = append(articles, page...)
		// Servers may cap pages below the requested size. Only an empty page
		// marks the end; use the actual count to avoid skipping articles.
		skip += len(page)
	}
}

// GetArticle fetches article content by its database ID or readable project ID.
func (c *Client) GetArticle(articleID string) (*Article, error) {
	if c.baseURL == "" || c.token == "" {
		return nil, errors.New("missing YouTrack connection URL or token")
	}
	articleID = strings.TrimSpace(articleID)
	if !validArticlePathID(articleID) {
		return nil, errors.New("a valid article ID is required")
	}
	query := url.Values{"fields": {articleDetailFields}}
	endpoint := strings.TrimRight(c.baseURL, "/") + "/api/articles/" + url.PathEscape(articleID) + "?" + query.Encode()
	body, err := c.readArticleResource(endpoint)
	if err != nil {
		return nil, err
	}
	var article Article
	if err := json.Unmarshal(body, &article); err != nil {
		return nil, fmt.Errorf("decode article: %w", err)
	}
	if article.ID == "" {
		return nil, errors.New("article response has no database ID")
	}
	return &article, nil
}

func validArticlePathID(id string) bool {
	return strings.TrimSpace(id) != "" && id != "." && id != ".."
}

func (c *Client) readArticleResource(endpoint string) ([]byte, error) {
	req, err := c.newRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	body, status, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, parseAPIError(status, body)
	}
	return body, nil
}
