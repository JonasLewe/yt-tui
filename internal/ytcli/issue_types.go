package ytcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// IssueTypeInfo describes the actual issue type field and values of a project.
// Values includes archived values because existing issues can still use them.
type IssueTypeInfo struct {
	Project   Project
	FieldName string
	Values    []string
	Found     bool
}

// ResolveIssueTypeProject resolves both configuration aliases before selecting
// the type field. The UI caches this identity together with the field metadata.
func (c *Client) ResolveIssueTypeProject(projectCode string) (Project, error) {
	if c.baseURL == "" || c.token == "" {
		return Project{}, errors.New("missing YouTrack connection URL or token")
	}
	if projectCode == "" {
		return Project{}, errors.New("project ID or short name is required")
	}
	if isIssueTypeProjectID(projectCode) {
		params := url.Values{"fields": {"id,shortName"}}
		endpoint := strings.TrimRight(c.baseURL, "/") + "/api/admin/projects/" + url.PathEscape(projectCode) + "?" + params.Encode()
		req, err := c.newRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			return Project{}, err
		}
		body, status, err := c.doRequest(req)
		if err != nil {
			return Project{}, err
		}
		if status != http.StatusOK {
			return Project{}, parseAPIError(status, body)
		}
		var project Project
		if err := json.Unmarshal(body, &project); err != nil {
			return Project{}, err
		}
		if project.ID != projectCode || project.ShortName == "" {
			return Project{}, errors.New("project response is missing its identity")
		}
		return project, nil
	}
	const pageSize = 200
	for skip := 0; ; {
		params := url.Values{"fields": {"id,shortName"}, "$top": {strconv.Itoa(pageSize)}, "$skip": {strconv.Itoa(skip)}}
		req, err := c.newRequest(http.MethodGet, strings.TrimRight(c.baseURL, "/")+"/api/admin/projects?"+params.Encode(), nil)
		if err != nil {
			return Project{}, err
		}
		body, status, err := c.doRequest(req)
		if err != nil {
			return Project{}, err
		}
		if status != http.StatusOK {
			return Project{}, parseAPIError(status, body)
		}
		var projects []Project
		if err := json.Unmarshal(body, &projects); err != nil {
			return Project{}, err
		}
		for _, project := range projects {
			if strings.EqualFold(project.ShortName, projectCode) && project.ID != "" {
				return project, nil
			}
		}
		if len(projects) < pageSize {
			return Project{}, fmt.Errorf("project with short name %s not found", projectCode)
		}
		skip += len(projects)
	}
}

// TypeValue reads the configured type field without inventing a default value.
// An empty fieldName tries Type, then Issue Type, case-insensitively.
func (i Issue) TypeValue(fieldName string) string {
	for _, candidate := range issueTypeFieldNames(fieldName) {
		for _, field := range i.CustomFields {
			if strings.EqualFold(field.Name, candidate) {
				if value := issueTypeValue(field.Value); value != "" {
					return value
				}
			}
		}
	}
	return ""
}

func issueTypeValue(value any) string {
	switch value := value.(type) {
	case string:
		return value
	case map[string]interface{}:
		name, _ := value["name"].(string)
		return name
	default:
		return ""
	}
}

func issueTypeFieldNames(configured string) []string {
	if configured = strings.TrimSpace(configured); configured != "" {
		return []string{configured}
	}
	return []string{"Type", "Issue Type"}
}

type projectIssueTypeField struct {
	Field struct {
		Name string `json:"name"`
	} `json:"field"`
	Type   string `json:"$type"`
	Bundle *struct {
		ID string `json:"id"`
	} `json:"bundle"`
}

type issueTypeBundleValue struct {
	Name    string `json:"name"`
	Ordinal int    `json:"ordinal"`
}

// GetProjectIssueTypes discovers a project's type field and its complete enum
// bundle. Missing fields return Found=false; failures remain errors so callers
// do not mistake inaccessible metadata for an empty set of types.
func (c *Client) GetProjectIssueTypes(projectID, configuredFieldName string) (IssueTypeInfo, error) {
	var info IssueTypeInfo
	if c.baseURL == "" || c.token == "" {
		return info, errors.New("missing YouTrack connection URL or token")
	}
	if projectID == "" {
		return info, errors.New("project ID is required")
	}
	// A short name can contain a hyphen; only numeric IDs are database IDs.
	if !isIssueTypeProjectID(projectID) {
		project, err := c.ResolveIssueTypeProject(projectID)
		if err != nil {
			return info, err
		}
		info.Project, projectID = project, project.ID
	} else {
		info.Project.ID = projectID
	}

	fields, err := fetchIssueTypePages[projectIssueTypeField](c,
		"api/admin/projects/"+url.PathEscape(projectID)+"/customFields",
		"field(name),$type,bundle(id)")
	if err != nil {
		return info, fmt.Errorf("load project issue type field: %w", err)
	}
	var selected *projectIssueTypeField
	for _, candidate := range issueTypeFieldNames(configuredFieldName) {
		for idx := range fields {
			if strings.EqualFold(fields[idx].Field.Name, candidate) {
				selected = &fields[idx]
				break
			}
		}
		if selected != nil {
			break
		}
	}
	if selected == nil {
		return info, nil
	}
	info.FieldName, info.Found = selected.Field.Name, true
	if selected.Type != "" && selected.Type != "EnumProjectCustomField" {
		return info, fmt.Errorf("issue type field %q is not an enum field", info.FieldName)
	}
	if selected.Bundle == nil || selected.Bundle.ID == "" {
		return info, fmt.Errorf("issue type field %q has no accessible enum bundle", info.FieldName)
	}
	values, err := fetchIssueTypePages[issueTypeBundleValue](c,
		"api/admin/customFieldSettings/bundles/enum/"+url.PathEscape(selected.Bundle.ID)+"/values",
		"name,ordinal")
	if err != nil {
		return info, fmt.Errorf("load issue types for %q: %w", info.FieldName, err)
	}
	sort.SliceStable(values, func(i, j int) bool {
		if values[i].Ordinal != values[j].Ordinal {
			return values[i].Ordinal < values[j].Ordinal
		}
		return values[i].Name < values[j].Name
	})
	seen := make(map[string]bool)
	for _, value := range values {
		key := strings.ToLower(value.Name)
		if strings.TrimSpace(value.Name) != "" && !seen[key] {
			info.Values = append(info.Values, value.Name)
			seen[key] = true
		}
	}
	return info, nil
}

func isIssueTypeProjectID(projectID string) bool {
	parts := strings.Split(projectID, "-")
	if len(parts) != 2 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				return false
			}
		}
	}
	return true
}

func fetchIssueTypePages[T any](c *Client, endpoint, fields string) ([]T, error) {
	const pageSize = 200
	var result []T
	for skip := 0; ; {
		params := url.Values{
			"fields": {fields},
			"$top":   {strconv.Itoa(pageSize)},
			"$skip":  {strconv.Itoa(skip)},
		}
		apiURL := strings.TrimRight(c.baseURL, "/") + "/" + endpoint + "?" + params.Encode()
		req, err := c.newRequest(http.MethodGet, apiURL, nil)
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
		var page []T
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, err
		}
		result = append(result, page...)
		if len(page) < pageSize {
			return result, nil
		}
		skip += len(page)
	}
}

// IssueTypeQuery builds an OR selection for a single enum field. YouTrack uses
// braces for values and commas for OR; its published grammar defines no escape
// for embedded braces. Reject unsupported literals instead of changing meaning.
// https://www.jetbrains.com/help/youtrack/cloud/attribute-based-search.html
func IssueTypeQuery(fieldName string, values []string) (string, error) {
	if len(values) == 0 {
		return "", nil
	}
	fieldName = strings.TrimSpace(fieldName)
	if err := validateIssueTypeLiteral(fieldName); err != nil {
		return "", fmt.Errorf("invalid issue type field: %w", err)
	}
	if strings.ContainsAny(fieldName, ":,()#\"'") {
		return "", errors.New("issue type field name contains unsupported query punctuation")
	}
	for _, word := range strings.Fields(fieldName) {
		if strings.EqualFold(word, "and") || strings.EqualFold(word, "or") {
			return "", errors.New("issue type field name contains a query operator")
		}
	}
	quoted := make([]string, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		if err := validateIssueTypeLiteral(value); err != nil {
			return "", fmt.Errorf("invalid issue type value: %w", err)
		}
		key := strings.ToLower(value)
		if !seen[key] {
			quoted = append(quoted, "{"+value+"}")
			seen[key] = true
		}
	}
	return fieldName + ": " + strings.Join(quoted, ", "), nil
}

func validateIssueTypeLiteral(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("empty names are not supported")
	}
	if strings.ContainsAny(value, "{}\\*?") {
		return errors.New("braces, backslashes and wildcard characters are not supported")
	}
	for _, ch := range value {
		if unicode.IsControl(ch) {
			return errors.New("control characters are not supported")
		}
	}
	return nil
}
