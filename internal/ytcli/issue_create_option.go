package ytcli

// IssueCreateOption adds project-specific fields or attributes to the creation request.
type IssueCreateOption func(payload map[string]interface{})
