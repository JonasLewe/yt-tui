package config

import (
	"encoding/json"
	"slices"
)

// These are frozen historical values, not references to today's defaults. An
// unversioned file could not distinguish a generated value from an identical
// explicit choice. The migration deliberately treats exactly these values as
// inherited, keeps every other choice, and backs up the original file first.
func migrateLegacy(original configDocument) configDocument {
	doc := make(configDocument, len(original)+1)
	for key, value := range original {
		doc[key] = slices.Clone(value)
	}
	doc["config_version"] = json.RawMessage("1")
	defaults := map[string]any{
		"page_size":        20,
		"max_issues":       500,
		"render_markdown":  true,
		"browser_command":  "xdg-open",
		"theme":            "catppuccin",
		"activity_filters": []string{"Comments"},
		"work_types":       []string{"Development", "Documentation", "Implementation", "Investigation", "Testing"},
	}
	for key, value := range defaults {
		if jsonEqual(doc[key], encodeValue(value)) {
			delete(doc, key)
		}
	}
	for _, fields := range [][]string{
		{"ID", "Summary", "State", "Priority", "Assignee"},
		{"ID", "Type", "Summary", "State", "Priority", "Assignee"},
	} {
		if jsonEqual(doc["fields"], encodeValue(fields)) {
			delete(doc, "fields")
		}
	}
	states := []string{"Open", "In Progress", "Verified", "Done", "Duplicate", "Won't fix", "Incomplete"}
	priorities := []string{"Minor", "Normal", "Major", "Critical", "Show-stopper"}
	pruneLegacyDefaultEntry(doc, "custom_states", states, false)
	pruneLegacyDefaultEntry(doc, "custom_priorities", priorities, false)
	pruneLegacyDefaultEntry(doc, "filtered_states", states, true)
	pruneLegacyDefaultEntry(doc, "filtered_priorities", priorities, true)
	for _, types := range [][]string{
		{"Bug", "Feature", "Task", "Epic", "Improvement", "Support"},
		{"Bug", "Feature", "Task", "Story", "Epic", "Improvement", "Support"},
	} {
		pruneLegacyDefaultEntry(doc, "custom_types", types, false)
	}
	return doc
}

func pruneLegacyDefaultEntry(doc configDocument, key string, expected []string, unordered bool) {
	object, ok := jsonObject(normalizeLegacyMap(key, doc[key]))
	if !ok {
		return
	}
	var value []string
	if err := json.Unmarshal(object["default"], &value); err != nil {
		return
	}
	if unordered {
		value = slices.Clone(value)
		expected = slices.Clone(expected)
		slices.Sort(value)
		slices.Sort(expected)
	}
	if !slices.Equal(value, expected) {
		return
	}
	delete(object, "default")
	if len(object) == 0 {
		delete(doc, key)
	} else {
		doc[key] = encodeValue(object)
	}
}
