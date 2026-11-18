package ytcli

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// IssueCreateFields describes the project's assignment field and available initial state choices.
type IssueCreateFields struct {
	AssigneeName     string
	AssigneeRequired bool
	AssigneeMulti    bool
	StateName        string
	States           []CreateState
}

type CreateState struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Archived bool   `json:"archived"`
}

// CreateIssueOptions carries selections special to a new issue. A nil Fields
// preserves the original clone API's treatment of an omitted assignee.
type CreateIssueOptions struct {
	Fields  *IssueCreateFields
	StateID string
	TagIDs  []string
}

// GetIssueCreateFields discovers field identity and multiplicity before any
// issue exists. Keep assignment metadata even if the state bundle is inaccessible.
func (c *Client) GetIssueCreateFields(projectID string) (IssueCreateFields, error) {
	var result IssueCreateFields
	if c == nil || c.baseURL == "" || c.token == "" || projectID == "" {
		return result, errors.New("project and YouTrack credentials are required")
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
		Bundle struct {
			ID string `json:"id"`
		} `json:"bundle"`
	}
	fields, err := fetchPages[projectField](c, "api/admin/projects/"+url.PathEscape(projectID)+"/customFields", "$type,canBeEmpty,field(name,fieldType(isMultiValue)),bundle(id)")
	if err != nil {
		// A user with limited access can still submit an ordinary issue using
		// the conventional single-user field and YouTrack's default state.
		result.AssigneeName = "Assignee"
		return result, err
	}
	var stateBundleID string
	for _, field := range fields {
		switch field.Type {
		case "UserProjectCustomField":
			if strings.EqualFold(field.Field.Name, "Assignee") {
				result.AssigneeName = field.Field.Name
				result.AssigneeRequired = !field.CanBeEmpty
				result.AssigneeMulti = field.Field.FieldType.Multi
			}
		case "StateProjectCustomField":
			if result.StateName == "" || strings.EqualFold(field.Field.Name, "State") {
				result.StateName = field.Field.Name
				stateBundleID = field.Bundle.ID
			}
		}
	}
	if result.StateName == "" {
		return result, nil
	}
	if stateBundleID == "" {
		return result, fmt.Errorf("state field %q has no accessible bundle", result.StateName)
	}
	states, err := fetchPages[CreateState](c, "api/admin/customFieldSettings/bundles/state/"+url.PathEscape(stateBundleID)+"/values", "id,name,archived")
	if err != nil {
		return result, err
	}
	for _, state := range states {
		if state.ID != "" && state.Name != "" && !state.Archived {
			result.States = append(result.States, state)
		}
	}
	return result, nil
}
