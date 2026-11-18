package ytcli

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

type ProjectAssignee struct {
	Name     string
	Required bool
	Multi    bool
}

// GetProjectAssignee returns the project's Assignee field. When project metadata
// is inaccessible, callers can still submit using YouTrack's conventional field.
func (c *Client) GetProjectAssignee(projectID string) (ProjectAssignee, error) {
	fallback := ProjectAssignee{Name: "Assignee"}
	if c == nil || c.baseURL == "" || c.token == "" || projectID == "" {
		return fallback, errors.New("project and YouTrack credentials are required")
	}
	type projectField struct {
		Type       string `json:"$type"`
		CanBeEmpty bool   `json:"canBeEmpty"`
		Field      struct {
			Name      string `json:"name"`
			FieldType struct {
				Multi bool `json:"isMultiValue"`
			} `json:"fieldType"`
		} `json:"field"`
	}
	fields, err := fetchPages[projectField](c, "api/admin/projects/"+url.PathEscape(projectID)+"/customFields",
		"$type,canBeEmpty,field(name,fieldType(isMultiValue))")
	if err != nil {
		return fallback, err
	}
	for _, field := range fields {
		if field.Type == "UserProjectCustomField" && strings.EqualFold(field.Field.Name, "Assignee") {
			return ProjectAssignee{Name: field.Field.Name, Required: !field.CanBeEmpty, Multi: field.Field.FieldType.Multi}, nil
		}
	}
	return ProjectAssignee{}, nil
}

// NewIssueAssignee builds the field for the initial POST, including an explicit
// empty value when a project would otherwise assign its default user.
func (c *Client) NewIssueAssignee(field ProjectAssignee, input string) (IssueCreateOption, error) {
	login := c.normalizeAssignee(input)
	if field.Name == "" && login != "" {
		return nil, errors.New("this project has no Assignee field")
	}
	if field.Required && login == "" {
		return nil, fmt.Errorf("%s is required in this project. Select a user", field.Name)
	}
	if login == "me" {
		var err error
		login, err = c.GetCurrentUserLogin()
		if err != nil {
			return nil, err
		}
	}
	return func(payload map[string]interface{}) {
		if field.Name == "" {
			return
		}
		var value any
		fieldType := "SingleUserIssueCustomField"
		if field.Multi {
			fieldType = "MultiUserIssueCustomField"
			users := []map[string]string{}
			if login != "" {
				users = append(users, map[string]string{"login": login})
			}
			value = users
		} else if login != "" {
			value = map[string]string{"login": login}
		}
		fields, _ := payload["customFields"].([]map[string]interface{})
		payload["customFields"] = append(fields, map[string]interface{}{"name": field.Name, "$type": fieldType, "value": value})
	}, nil
}
