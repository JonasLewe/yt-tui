package ytcli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const tagFields = "id,name,owner(id,login)"

// ListTags retrieves the complete set of tags visible to this account. It is
// deliberately independent of issue pagination and the configured issue limit.
// https://www.jetbrains.com/help/youtrack/devportal/resource-api-tags.html
func (c *Client) ListTags() ([]Tag, error) {
	if c == nil || c.baseURL == "" || c.token == "" {
		return nil, errors.New("missing YouTrack connection URL or token")
	}
	const pageSize = 200
	var result []Tag
	seen := make(map[string]bool)
	for skip := 0; ; {
		params := url.Values{
			"fields": {tagFields},
			"$top":   {strconv.Itoa(pageSize)},
			"$skip":  {strconv.Itoa(skip)},
		}
		endpoint := strings.TrimRight(c.baseURL, "/") + "/api/tags?" + params.Encode()
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
		var page []Tag
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("decode tags: %w", err)
		}
		if len(page) == 0 {
			return result, nil
		}
		previousCount := len(result)
		for _, tag := range page {
			if tag.ID == "" || strings.TrimSpace(tag.Name) == "" {
				return nil, errors.New("tag response is missing an ID or name")
			}
			if !seen[tag.ID] {
				result = append(result, tag)
				seen[tag.ID] = true
			}
		}
		if len(result) == previousCount {
			return nil, errors.New("tag pagination made no progress")
		}
		// Some installations cap page lengths below the requested $top. Only an
		// empty page proves exhaustion; advance by the actual number returned.
		skip += len(page)
	}
}

// TagQuery selects one or more tag names (OR). YouTrack's documented issue
// query grammar accepts names, not tag IDs or owner qualifiers. Equal names
// therefore share one option, even if the API exposes several distinct tags.
// Unsupported literals fail explicitly instead of widening the query.
// https://www.jetbrains.com/help/youtrack/cloud/search-and-command-attributes.html
func TagQuery(names []string) (string, error) {
	if len(names) == 0 {
		return "", nil
	}
	quoted := make([]string, 0, len(names))
	seen := make(map[string]bool)
	for _, name := range names {
		if err := validateIssueTypeLiteral(name); err != nil {
			return "", fmt.Errorf("invalid tag name: %w", err)
		}
		key := strings.ToLower(name)
		if !seen[key] {
			quoted = append(quoted, "{"+name+"}")
			seen[key] = true
		}
	}
	return "tag: " + strings.Join(quoted, ", "), nil
}

// ContextKey identifies the current credentials for in-memory caches and
// asynchronous response guards, including account changes on the same server.
// The opaque fingerprint never exposes the token and is not a persistent ID.
func (c *Client) ContextKey() string {
	if c == nil {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.TrimRight(c.baseURL, "/") + "\x00" + c.token))
	return hex.EncodeToString(sum[:])
}

// TagFilterScope resolves a stable server/account key for saved tag filters.
// Tokens may rotate without losing preferences. Identity failures must not be
// treated as an unrestricted filter or a fallback to another account's data.
func (c *Client) TagFilterScope() (string, error) {
	if c == nil || c.baseURL == "" || c.token == "" {
		return "", errors.New("missing YouTrack connection URL or token")
	}
	server, err := url.Parse(c.baseURL)
	if err != nil || server.Scheme == "" || server.Host == "" {
		return "", errors.New("invalid YouTrack connection URL")
	}
	server.Scheme = strings.ToLower(server.Scheme)
	server.Host = strings.ToLower(server.Host)
	server.User, server.RawQuery, server.Fragment = nil, "", ""
	server.Path = strings.TrimRight(server.Path, "/")
	server.RawPath = strings.TrimRight(server.RawPath, "/")

	endpoint := strings.TrimRight(c.baseURL, "/") + "/api/users/me?fields=id,login"
	req, err := c.newRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	body, status, err := c.doRequest(req)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", parseAPIError(status, body)
	}
	var user User
	if err := json.Unmarshal(body, &user); err != nil {
		return "", fmt.Errorf("decode current user: %w", err)
	}
	if user.ID == "" {
		return "", errors.New("received empty ID for current user")
	}
	return server.String() + "#" + url.PathEscape(user.ID), nil
}
