package ytcli

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

type InitialState struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Archived bool   `json:"archived"`
}

type ProjectStates struct {
	FieldName string
	Values    []InitialState
}

// GetProjectStates returns active values from the project's state bundle.
func (c *Client) GetProjectStates(projectID string) (ProjectStates, error) {
	var result ProjectStates
	if c == nil || c.baseURL == "" || c.token == "" || projectID == "" {
		return result, errors.New("project and YouTrack credentials are required")
	}
	fields, err := fetchPages[projectIssueTypeField](c,
		"api/admin/projects/"+url.PathEscape(projectID)+"/customFields", "$type,field(name),bundle(id)")
	if err != nil {
		return result, err
	}
	var state *projectIssueTypeField
	for i, field := range fields {
		if field.Type == "StateProjectCustomField" && field.Field.Name != "" {
			if state == nil || strings.EqualFold(field.Field.Name, "State") {
				state = &fields[i]
			}
		}
	}
	if state == nil {
		return result, nil
	}
	result.FieldName = state.Field.Name
	if state.Bundle == nil || state.Bundle.ID == "" {
		return result, fmt.Errorf("state field %q has no accessible bundle", result.FieldName)
	}
	values, err := fetchPages[InitialState](c,
		"api/admin/customFieldSettings/bundles/state/"+url.PathEscape(state.Bundle.ID)+"/values", "id,name,archived")
	if err != nil {
		return result, err
	}
	for _, value := range values {
		if value.ID != "" && value.Name != "" && !value.Archived {
			result.Values = append(result.Values, value)
		}
	}
	return result, nil
}

func WithInitialState(fieldName, stateID string) IssueCreateOption {
	return func(payload map[string]interface{}) {
		fields, _ := payload["customFields"].([]map[string]interface{})
		payload["customFields"] = append(fields, map[string]interface{}{
			"name": fieldName, "$type": "StateIssueCustomField", "value": map[string]string{"id": stateID},
		})
	}
}
