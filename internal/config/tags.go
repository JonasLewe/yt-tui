package config

import (
	"maps"
	"slices"
)

// FilteredTagsMap stores tag names by authenticated server/account and project.
type FilteredTagsMap map[string]map[string][]string

// GetFilteredTags returns a name selection for an authenticated server/account
// scope. Project aliases are checked in order (short name, ID, original input),
// followed by that scope's default. An explicit empty list means unrestricted.
// No selection is ever inherited from another server or account.
func (cfg *Config) GetFilteredTags(scope string, projectCodes ...string) []string {
	if cfg == nil || scope == "" {
		return nil
	}
	projects := cfg.FilteredTags[scope]
	for _, project := range projectCodes {
		if tags, ok := projects[project]; project != "" && ok && tags != nil {
			return slices.Clone(tags)
		}
	}
	return slices.Clone(projects["default"])
}

// SetFilteredTags records only this project selection. Copy-on-write protects
// an original config when a shallow UI draft fails to save. Empty project uses
// the account-scoped default; an unresolved account scope cannot be written.
func (cfg *Config) SetFilteredTags(scope, projectCode string, names []string) {
	if cfg == nil || scope == "" {
		return
	}
	if projectCode == "" {
		projectCode = "default"
	}
	all := maps.Clone(cfg.FilteredTags)
	if all == nil {
		all = make(map[string]map[string][]string)
	}
	projects := maps.Clone(all[scope])
	if projects == nil {
		projects = make(map[string][]string)
	}
	projects[projectCode] = append([]string{}, names...)
	all[scope] = projects
	cfg.FilteredTags = all
	cfg.markOverride("filtered_tags", scope, projectCode)
}
