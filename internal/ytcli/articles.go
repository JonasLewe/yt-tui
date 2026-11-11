package ytcli

import (
	"bytes"
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

// CreateArticle creates an article in a project, optionally beneath a parent,
// and returns its readable ID (or database ID when no readable ID is returned).
func (c *Client) CreateArticle(projectCode, summary, content, parentID string) (string, error) {
	if c.baseURL == "" || c.token == "" {
		return "", errors.New("missing YouTrack connection URL or token")
	}
	projectCode = strings.TrimSpace(projectCode)
	if projectCode == "" {
		return "", errors.New("project short name is required")
	}
	if strings.TrimSpace(summary) == "" {
		return "", errors.New("article summary is required")
	}
	if parentID != "" {
		parentID = strings.TrimSpace(parentID)
		if !validArticlePathID(parentID) {
			return "", errors.New("a valid parent article ID is required")
		}
	}
	projectID, err := c.getProjectIDByShortName(projectCode)
	if err != nil {
		return "", err
	}
	if !validArticlePathID(projectID) {
		return "", errors.New("project has no valid database ID")
	}
	payload := map[string]interface{}{
		"project": map[string]string{"id": projectID},
		"summary": summary,
		"content": content,
	}
	if parentID != "" {
		payload["parentArticle"] = map[string]string{"id": parentID}
	}
	endpoint := strings.TrimRight(c.baseURL, "/") + "/api/articles?fields=id,idReadable"
	body, status, err := c.writeArticleResource(endpoint, payload)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return "", parseAPIError(status, body)
	}
	var created Article
	if err := json.Unmarshal(body, &created); err != nil {
		return "", fmt.Errorf("decode created article: %w", err)
	}
	if created.IDReadable != "" {
		return created.IDReadable, nil
	}
	if created.ID == "" {
		return "", errors.New("article creation response has no ID")
	}
	return created.ID, nil
}

// UpdateArticle changes only an article's title and content. Project, parent,
// visibility, and all other article attributes remain untouched.
func (c *Client) UpdateArticle(articleID, summary, content string) error {
	if c.baseURL == "" || c.token == "" {
		return errors.New("missing YouTrack connection URL or token")
	}
	articleID = strings.TrimSpace(articleID)
	if !validArticlePathID(articleID) {
		return errors.New("a valid article ID is required")
	}
	if strings.TrimSpace(summary) == "" {
		return errors.New("article summary is required")
	}
	payload := map[string]interface{}{"summary": summary, "content": content}
	endpoint := strings.TrimRight(c.baseURL, "/") + "/api/articles/" + url.PathEscape(articleID)
	body, status, err := c.writeArticleResource(endpoint, payload)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		return parseAPIError(status, body)
	}
	return nil
}

func (c *Client) writeArticleResource(endpoint string, payload map[string]interface{}) ([]byte, int, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	req, err := c.newRequest(http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, 0, err
	}
	return c.doRequest(req)
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
