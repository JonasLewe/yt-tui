package ytcli

// WithIssueTags assigns the selected native YouTrack tags in the issue creation request.
func WithIssueTags(ids []string) IssueCreateOption {
	return func(payload map[string]interface{}) {
		tags := make([]map[string]string, 0, len(ids))
		for _, id := range ids {
			tags = append(tags, map[string]string{"id": id})
		}
		payload["tags"] = tags
	}
}
