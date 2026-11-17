package config

import (
	"encoding/json"
	"fmt"
)

// CustomStatesMap represents custom states configured per project, or a default fallback.
type CustomStatesMap map[string][]string

// UnmarshalJSON handles loading custom_states either as a list of strings (legacy/fallback) or a map of list of strings.
func (c *CustomStatesMap) UnmarshalJSON(data []byte) error {
	// Try unmarshaling as map first
	var m map[string][]string
	if err := json.Unmarshal(data, &m); err == nil {
		*c = m
		return nil
	}

	// If that fails, try unmarshaling as a slice of strings
	var s []string
	if err := json.Unmarshal(data, &s); err == nil {
		*c = map[string][]string{
			"default": s,
		}
		return nil
	}

	return fmt.Errorf("custom_states must be either a list of strings or a map of list of strings")
}

// CustomPrioritiesMap represents custom priorities configured per project, or a default fallback.
type CustomPrioritiesMap map[string][]string

// UnmarshalJSON handles loading custom_priorities either as a list of strings (legacy/fallback) or a map of list of strings.
func (c *CustomPrioritiesMap) UnmarshalJSON(data []byte) error {
	// Try unmarshaling as map first
	var m map[string][]string
	if err := json.Unmarshal(data, &m); err == nil {
		*c = m
		return nil
	}

	// If that fails, try unmarshaling as a slice of strings
	var s []string
	if err := json.Unmarshal(data, &s); err == nil {
		*c = map[string][]string{
			"default": s,
		}
		return nil
	}

	return fmt.Errorf("custom_priorities must be either a list of strings or a map of list of strings")
}

// CustomTypesMap represents custom types configured per project, or a default fallback.
type CustomTypesMap map[string][]string

// UnmarshalJSON handles loading custom_types either as a list of strings (legacy/fallback) or a map of list of strings.
func (c *CustomTypesMap) UnmarshalJSON(data []byte) error {
	// Try unmarshaling as map first
	var m map[string][]string
	if err := json.Unmarshal(data, &m); err == nil {
		*c = m
		return nil
	}

	// If that fails, try unmarshaling as a slice of strings
	var s []string
	if err := json.Unmarshal(data, &s); err == nil {
		*c = map[string][]string{
			"default": s,
		}
		return nil
	}

	return fmt.Errorf("custom_types must be either a list of strings or a map of list of strings")
}

// FilteredStatesMap represents filtered states configured per project, or a default fallback.
type FilteredStatesMap map[string][]string

// UnmarshalJSON handles loading filtered_states either as a list of strings (legacy/fallback) or a map of list of strings.
func (f *FilteredStatesMap) UnmarshalJSON(data []byte) error {
	// Try unmarshaling as map first
	var m map[string][]string
	if err := json.Unmarshal(data, &m); err == nil {
		*f = m
		return nil
	}

	// If that fails, try unmarshaling as a slice of strings
	var s []string
	if err := json.Unmarshal(data, &s); err == nil {
		*f = map[string][]string{
			"default": s,
		}
		return nil
	}

	return fmt.Errorf("filtered_states must be either a list of strings or a map of list of strings")
}

// FilteredPrioritiesMap represents filtered priorities configured per project, or a default fallback.
type FilteredPrioritiesMap map[string][]string

// UnmarshalJSON handles loading filtered_priorities either as a list of strings (legacy/fallback) or a map of list of strings.
func (f *FilteredPrioritiesMap) UnmarshalJSON(data []byte) error {
	// Try unmarshaling as map first
	var m map[string][]string
	if err := json.Unmarshal(data, &m); err == nil {
		*f = m
		return nil
	}

	// If that fails, try unmarshaling as a slice of strings
	var s []string
	if err := json.Unmarshal(data, &s); err == nil {
		*f = map[string][]string{
			"default": s,
		}
		return nil
	}

	return fmt.Errorf("filtered_priorities must be either a list of strings or a map of list of strings")
}

// ActionsMap represents custom actions configured per project, or a default fallback.
type ActionsMap map[string][]ActionConfig

// UnmarshalJSON handles loading actions either as a list of objects (legacy/fallback) or a map of list of objects.
func (a *ActionsMap) UnmarshalJSON(data []byte) error {
	// Try unmarshaling as map first
	var m map[string][]ActionConfig
	if err := json.Unmarshal(data, &m); err == nil {
		*a = m
		return nil
	}

	// If that fails, try unmarshaling as a slice of ActionConfig
	var s []ActionConfig
	if err := json.Unmarshal(data, &s); err == nil {
		*a = map[string][]ActionConfig{
			"default": s,
		}
		return nil
	}

	return fmt.Errorf("actions must be either a list of objects or a map of list of objects")
}

// Config represents the application configuration.
type Config struct {
	// Tracking is immutable: a shallow Config copy can be saved independently.
	tracking            *configTracking
	explicitPaths       [][]string
	URL                 string                `json:"url"`
	Token               string                `json:"token"`
	Servers             []ServerConfig        `json:"servers,omitempty"`
	PageSize            int                   `json:"page_size"`
	MaxIssues           int                   `json:"max_issues"`
	Fields              []string              `json:"fields"`
	CustomTypes         CustomTypesMap        `json:"custom_types"`
	CustomPriorities    CustomPrioritiesMap   `json:"custom_priorities"`
	CustomStates        CustomStatesMap       `json:"custom_states"`
	WorkTypes           []string              `json:"work_types"`
	FilteredStates      FilteredStatesMap     `json:"filtered_states"`
	FilteredPriorities  FilteredPrioritiesMap `json:"filtered_priorities"`
	FilteredTypes       map[string][]string   `json:"filtered_types,omitempty"`
	TypeFieldName       map[string]string     `json:"type_field_name,omitempty"`
	SortColumn          string                `json:"sort_column"`
	SortDirection       string                `json:"sort_direction"`
	FavoriteView        string                `json:"favorite_view,omitempty"`
	FavoriteViews       map[string]string     `json:"favorite_views,omitempty"`
	ActivityFilters     []string              `json:"activity_filters,omitempty"`
	ActivitySortOrder   string                `json:"activity_sort_order,omitempty"`
	RenderMarkdown      bool                  `json:"render_markdown"`
	RepoOptions         map[string][]string   `json:"repo_options,omitempty"`
	BoardsOptions       map[string][]string   `json:"boards_options,omitempty"`
	BoardsFieldName     map[string]string     `json:"boards_field_name,omitempty"`
	FilepickerSortBy    string                `json:"filepicker_sort_by,omitempty"`
	FilepickerSortOrder string                `json:"filepicker_sort_order,omitempty"`
	FilepickerLastDir   string                `json:"filepicker_last_dir,omitempty"`
	Actions             ActionsMap            `json:"actions,omitempty"`
	ImageViewer         string                `json:"image_viewer,omitempty"`
	VcsBaseURL          string                `json:"vcs_base_url,omitempty"`
	BrowserCommand      string                `json:"browser_command,omitempty"`
	GitLabCommand       string                `json:"gitlab_command,omitempty"`
	Theme               string                `json:"theme"`
	UsernameSeparator   string                `json:"username_separator,omitempty"`
}

// GetCustomStates returns the list of custom states for the given project.
// If projectCode is empty or not found, it falls back to the "default" key,
// and finally to config.DefaultStates.
func (cfg *Config) GetCustomStates(projectCode string) []string {
	if cfg == nil || len(cfg.CustomStates) == 0 {
		return DefaultStates
	}
	if projectCode != "" {
		if states, ok := cfg.CustomStates[projectCode]; ok {
			return states
		}
	}
	if states, ok := cfg.CustomStates["default"]; ok {
		return states
	}
	return DefaultStates
}

// GetCustomPriorities returns the list of custom priorities for the given project.
// If projectCode is empty or not found, it falls back to the "default" key,
// and finally to config.DefaultPriorities.
func (cfg *Config) GetCustomPriorities(projectCode string) []string {
	if cfg == nil || len(cfg.CustomPriorities) == 0 {
		return DefaultPriorities
	}
	if projectCode != "" {
		if priorities, ok := cfg.CustomPriorities[projectCode]; ok {
			return priorities
		}
	}
	if priorities, ok := cfg.CustomPriorities["default"]; ok {
		return priorities
	}
	return DefaultPriorities
}

// GetBoardsFieldName returns the configured boards/sprint custom field name for a project.
func (cfg *Config) GetBoardsFieldName(projectCode string) string {
	if cfg == nil || len(cfg.BoardsFieldName) == 0 {
		return ""
	}
	if projectCode != "" {
		if name, ok := cfg.BoardsFieldName[projectCode]; ok {
			return name
		}
	}
	if name, ok := cfg.BoardsFieldName["default"]; ok && name != "" {
		return name
	}
	return ""
}

// GetTypeFieldName returns the configured issue type field name for a project.
// An empty result lets the caller detect the standard Type or Issue Type field.
func (cfg *Config) GetTypeFieldName(projectCodes ...string) string {
	if cfg == nil {
		return ""
	}
	for _, projectCode := range projectCodes {
		if name, ok := cfg.TypeFieldName[projectCode]; projectCode != "" && ok {
			return name
		}
	}
	return cfg.TypeFieldName["default"]
}

// GetCustomTypes returns the list of custom types for the given project.
// If projectCode is empty or not found, it falls back to the "default" key,
// and finally to config.DefaultTypes.
func (cfg *Config) GetCustomTypes(projectCode string) []string {
	if cfg == nil || len(cfg.CustomTypes) == 0 {
		return DefaultTypes
	}
	if projectCode != "" {
		if types, ok := cfg.CustomTypes[projectCode]; ok {
			return types
		}
	}
	if types, ok := cfg.CustomTypes["default"]; ok {
		return types
	}
	return DefaultTypes
}

// GetFilteredStates returns the list of filtered states for the given project.
func (cfg *Config) GetFilteredStates(projectCode string) []string {
	if cfg == nil || len(cfg.FilteredStates) == 0 {
		return cfg.GetCustomStates(projectCode)
	}
	key := projectCode
	if key == "" {
		key = "default"
	}
	if states, ok := cfg.FilteredStates[key]; ok {
		return states
	}
	if states, ok := cfg.FilteredStates["default"]; ok {
		return states
	}
	return cfg.GetCustomStates(projectCode)
}

// GetFilteredPriorities returns the list of filtered priorities for the given project.
func (cfg *Config) GetFilteredPriorities(projectCode string) []string {
	if cfg == nil || len(cfg.FilteredPriorities) == 0 {
		return cfg.GetCustomPriorities(projectCode)
	}
	key := projectCode
	if key == "" {
		key = "default"
	}
	if priorities, ok := cfg.FilteredPriorities[key]; ok {
		return priorities
	}
	if priorities, ok := cfg.FilteredPriorities["default"]; ok {
		return priorities
	}
	return cfg.GetCustomPriorities(projectCode)
}

// GetFilteredTypes returns selected issue types, or no restriction if empty.
// A project entry overrides the default even when its list is empty.
func (cfg *Config) GetFilteredTypes(projectCodes ...string) []string {
	if cfg == nil {
		return nil
	}
	for _, projectCode := range projectCodes {
		if types, ok := cfg.FilteredTypes[projectCode]; projectCode != "" && ok {
			return types
		}
	}
	return cfg.FilteredTypes["default"]
}

// GetActions returns the list of custom actions for the given project.
// If projectCode is empty or not found, it falls back to the "default" key,
// and finally to nil.
func (cfg *Config) GetActions(projectCode string) []ActionConfig {
	if cfg == nil || len(cfg.Actions) == 0 {
		return nil
	}
	if projectCode != "" {
		if actions, ok := cfg.Actions[projectCode]; ok {
			return actions
		}
	}
	if actions, ok := cfg.Actions["default"]; ok {
		return actions
	}
	return nil
}

// SetFilteredTypes saves the type selection for a project, using an empty list
// to explicitly select all types even when a default filter is configured.
func (cfg *Config) SetFilteredTypes(projectCode string, types []string) {
	if cfg == nil {
		return
	}
	if cfg.FilteredTypes == nil {
		cfg.FilteredTypes = make(map[string][]string)
	}
	if projectCode == "" {
		projectCode = "default"
	}
	cfg.FilteredTypes[projectCode] = append([]string{}, types...)
	cfg.markOverride("filtered_types", projectCode)
}

// SetFilteredStates sets the filtered states for the given project.
func (cfg *Config) SetFilteredStates(projectCode string, states []string) {
	if cfg == nil {
		return
	}
	if cfg.FilteredStates == nil {
		cfg.FilteredStates = make(FilteredStatesMap)
	}
	key := projectCode
	if key == "" {
		key = "default"
	}
	cfg.FilteredStates[key] = append([]string{}, states...)
	cfg.markOverride("filtered_states", key)
}

// SetFilteredPriorities sets the filtered priorities for the given project.
func (cfg *Config) SetFilteredPriorities(projectCode string, priorities []string) {
	if cfg == nil {
		return
	}
	if cfg.FilteredPriorities == nil {
		cfg.FilteredPriorities = make(FilteredPrioritiesMap)
	}
	key := projectCode
	if key == "" {
		key = "default"
	}
	cfg.FilteredPriorities[key] = append([]string{}, priorities...)
	cfg.markOverride("filtered_priorities", key)
}

// GetFavoriteView returns the favorite view for the given server URL.
// If the server URL is not in the map, it falls back to the top-level FavoriteView
// only if no other per-server favorites have been configured yet (migration fallback).
func (cfg *Config) GetFavoriteView(url string) string {
	if cfg == nil {
		return ""
	}
	if cfg.FavoriteViews != nil {
		if fav, ok := cfg.FavoriteViews[url]; ok {
			return fav
		}
		if len(cfg.FavoriteViews) > 0 {
			// Per-server favorites are being used, do not fall back to the global one
			return ""
		}
	}
	return cfg.FavoriteView
}

// SetFavoriteView sets the favorite view for the given server URL.
func (cfg *Config) SetFavoriteView(url string, view string) {
	if cfg == nil {
		return
	}
	cfg.markOverride("favorite_views", url)
	cfg.markOverride("favorite_view")
	if cfg.FavoriteViews == nil {
		cfg.FavoriteViews = make(map[string]string)
	}
	if view == "" {
		delete(cfg.FavoriteViews, url)
		if cfg.FavoriteView != "" && len(cfg.FavoriteViews) == 0 {
			cfg.FavoriteView = ""
		}
	} else {
		cfg.FavoriteViews[url] = view
		cfg.FavoriteView = view
	}
}

type ActionCommand struct {
	Type   string   `json:"type"`
	Field  string   `json:"field,omitempty"`
	Value  string   `json:"value"`
	Values []string `json:"values,omitempty"`
}

type ActionConfig struct {
	Name     string          `json:"name"`
	Shortcut string          `json:"shortcut"`
	Commands []ActionCommand `json:"commands"`
}

type ServerConfig struct {
	Name              string `json:"name,omitempty"`
	URL               string `json:"url"`
	Token             string `json:"token"`
	VcsBaseURL        string `json:"vcs_base_url,omitempty"`
	UsernameSeparator string `json:"username_separator,omitempty"`
}

const (
	DefaultPageSize  = 20
	DefaultMaxIssues = 500
)

var (
	DefaultFields     = []string{"ID", "Type", "Summary", "State", "Priority", "Assignee"}
	DefaultTypes      = []string{"Bug", "Feature", "Task", "Story", "Epic", "Improvement", "Support"}
	DefaultPriorities = []string{"Minor", "Normal", "Major", "Critical", "Show-stopper"}
	DefaultStates     = []string{"Open", "In Progress", "Verified", "Done", "Duplicate", "Won't fix", "Incomplete"}
	DefaultWorkTypes  = []string{"Development", "Documentation", "Implementation", "Investigation", "Testing"}
)
