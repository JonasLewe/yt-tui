package ui

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"yt-tui/internal/config"
	"yt-tui/internal/ytcli"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type issueTypesKey struct {
	serverURL, projectCode, fieldName, fieldConfig string
}

type issueTypesMsg struct {
	key       issueTypesKey
	requestID uint64
	info      ytcli.IssueTypeInfo
	err       error
}

func issueTypeLabel(issue ytcli.Issue, cfg *config.Config, projectCode string) string {
	aliases := []string{projectCode}
	if issue.Project != nil {
		aliases = []string{issue.Project.ShortName, issue.Project.ID, projectCode}
	}
	value := issue.TypeValue(cfg.GetTypeFieldName(aliases...))
	if value == "" {
		return "—"
	}
	return value
}

func (m issuesModel) typeKey() issueTypesKey {
	var fields map[string]string
	if m.cfg != nil {
		fields = m.cfg.TypeFieldName
	}
	// Encoding is deterministic for string-keyed maps. Changing an ID-keyed
	// mapping must also invalidate metadata cached under the short name.
	encoded, _ := json.Marshal(fields)
	return issueTypesKey{m.serverURL, m.projectCode, m.cfg.GetTypeFieldName(m.projectCode), string(encoded)}
}

func (m issuesModel) projectAliases() []string {
	return []string{m.typeInfo.Project.ShortName, m.typeInfo.Project.ID, m.projectCode}
}

func (m issuesModel) sameServer() bool {
	return m.client == nil || m.serverURL == m.client.GetConfiguredBaseURL()
}

// Each context transition gets a generation, including A -> B -> A and refreshes
// of the same query. Query equality alone cannot reject responses from an old run.
func (m *issuesModel) startContext(refresh bool) tea.Cmd {
	m.requestID++
	m.serverURL = ""
	if m.client != nil {
		m.serverURL = m.client.GetConfiguredBaseURL()
	}
	m.selectedTypes = nil
	if m.projectCode != "" {
		m.selectedTypes = normalizedTypes(m.cfg.GetFilteredTypes(m.projectCode))
	}
	m.typeInfo, m.typeInfoErr = ytcli.IssueTypeInfo{}, nil
	m.err, m.loadingText = nil, ""
	m.issues, m.skip, m.loadedAll, m.limitReached = nil, 0, false, false
	m.loading = true
	m.updateTableRows()
	if refresh {
		m.invalidateCache(m.projectCode)
		delete(m.typeInfoCache, m.typeKey())
	}
	if m.projectCode != "" {
		if info, ok := m.typeInfoCache[m.typeKey()]; ok {
			m.typeInfo = info
		} else if m.client != nil {
			msg := issueTypesMsg{key: m.typeKey(), requestID: m.requestID}
			client := *m.client
			var fieldNames map[string]string
			if m.cfg != nil {
				fieldNames = maps.Clone(m.cfg.TypeFieldName)
			}
			return func() tea.Msg {
				project, err := client.ResolveIssueTypeProject(msg.key.projectCode)
				if err != nil {
					msg.err = fmt.Errorf("resolve issue type project: %w", err)
					return msg
				}
				cfg := &config.Config{TypeFieldName: fieldNames}
				fieldName := cfg.GetTypeFieldName(project.ShortName, project.ID)
				msg.info, msg.err = client.GetProjectIssueTypes(project.ID, fieldName)
				msg.info.Project = project
				return msg
			}
		}
	}
	return m.restoreOrLoadIssues()
}

func (m *issuesModel) restoreOrLoadIssues() tea.Cmd {
	if m.projectCode != "" {
		previous := m.selectedTypes
		m.selectedTypes = canonicalTypeValues(m.cfg.GetFilteredTypes(m.projectAliases()...), m.typeInfo.Values)
		if m.filterMode && len(m.filterPanel.sections) == 3 {
			section := &m.filterPanel.sections[2]
			if slices.Equal(normalizedTypes(selectedValues(section.Options, section.Selected)), previous) {
				section.Selected = selection(m.selectedTypes)
			}
			section.Options = m.availableTypes()
		}
	}
	m.serverQuery = m.query
	if m.unresolvedSavedTypeFilter() {
		m.err = fmt.Errorf("cannot resolve saved project type filters: %w", m.typeInfoErr)
		m.loading = false
		return nil
	}
	if len(m.selectedTypes) > 0 {
		if m.typeInfoErr != nil {
			m.err = fmt.Errorf("cannot apply type filter: %w", m.typeInfoErr)
		} else if !m.typeInfo.Found {
			m.err = fmt.Errorf("no issue type field found; choose All types or configure type_field_name")
		} else {
			var clause string
			clause, m.err = ytcli.IssueTypeQuery(m.typeInfo.FieldName, m.selectedTypes)
			if m.query == "" {
				m.serverQuery = clause
			} else {
				m.serverQuery = "(" + m.query + ") and (" + clause + ")"
			}
		}
		if m.err != nil {
			m.loading = false
			return nil
		}
	}
	if cached, ok := m.cache[m.cacheKey()]; ok {
		m.issues = append([]ytcli.Issue(nil), cached.issues...)
		m.skip, m.loadedAll, m.limitReached = cached.skip, cached.loadedAll, cached.limitReached
		m.loading = false
		m.updateTableRows()
		m.restoreCursor()
		if m.loadedAll {
			return nil
		}
	}
	return m.loadIssuesCmd()
}

// If identity lookup failed, an ID-keyed filter may belong to this short name.
// Do not mistake an unresolved alias for an unfiltered project. An explicit
// All entry for the current view remains a recovery path without metadata.
func (m issuesModel) unresolvedSavedTypeFilter() bool {
	if m.projectCode == "" || m.typeInfoErr == nil || m.typeInfo.Project.ID != "" || m.cfg == nil {
		return false
	}
	if _, explicit := m.cfg.FilteredTypes[m.projectCode]; explicit {
		return false
	}
	for code, values := range m.cfg.FilteredTypes {
		if code != "default" && len(values) > 0 {
			return true
		}
	}
	return false
}

func normalizedTypes(values []string) []string {
	var result []string
	seen := make(map[string]bool)
	for _, value := range values {
		key := strings.ToLower(value)
		if !seen[key] {
			result = append(result, value)
			seen[key] = true
		}
	}
	sort.Slice(result, func(i, j int) bool { return strings.ToLower(result[i]) < strings.ToLower(result[j]) })
	return result
}

func canonicalTypeValues(values, serverValues []string) []string {
	canonical := make(map[string]string, len(serverValues))
	for _, value := range serverValues {
		canonical[strings.ToLower(value)] = value
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if label, ok := canonical[strings.ToLower(value)]; ok {
			value = label
		}
		result = append(result, value)
	}
	return normalizedTypes(result)
}

func selectedValues(options []string, selected map[string]bool) []string {
	values := []string{}
	seen := make(map[string]bool)
	for _, option := range options {
		if selected[option] && !seen[option] {
			values = append(values, option)
			seen[option] = true
		}
	}
	var extras []string
	for value, yes := range selected {
		if yes && !seen[value] {
			extras = append(extras, value)
		}
	}
	sort.Strings(extras)
	return append(values, extras...)
}

func selection(values []string) map[string]bool {
	selected := make(map[string]bool, len(values))
	for _, value := range values {
		selected[value] = true
	}
	return selected
}

func (m issuesModel) availableTypes() []string {
	values := append([]string(nil), m.typeInfo.Values...)
	// Always retain saved and observed values, including archived values. On a
	// metadata error this is explicitly a partial selection, not a complete bundle.
	values = append(values, m.selectedTypes...)
	for _, issue := range m.issues {
		value := issue.TypeValue(m.cfg.GetTypeFieldName(m.projectAliases()...))
		if value != "" {
			values = append(values, value)
		}
	}
	return normalizedTypes(values)
}

func (m *issuesModel) openFilters() {
	if m.cfg == nil {
		return
	}
	code := m.effectiveProjectCode()
	sections := []issueFilterSection{
		{Title: "States", Options: m.getAvailableStates(), Selected: selection(m.cfg.GetFilteredStates(code))},
		{Title: "Priorities", Options: m.getAvailablePriorities(), Selected: selection(m.cfg.GetFilteredPriorities(code))},
	}
	if m.projectCode != "" {
		sections = append(sections, issueFilterSection{
			Title: "Types", Options: m.availableTypes(), Selected: selection(m.selectedTypes), AllowAll: true,
		})
	}
	m.filterPanel = newIssuesFilterPanel(sections)
	m.filterMode, m.filterError = true, ""
}

func (m *issuesModel) saveFilters() tea.Cmd {
	if m.cfg == nil || len(m.filterPanel.sections) < 2 {
		return nil
	}
	code := m.effectiveProjectCode()
	// Save a detached candidate. A failed write must not apply half the filters.
	next := *m.cfg
	next.FilteredStates = maps.Clone(m.cfg.FilteredStates)
	next.FilteredPriorities = maps.Clone(m.cfg.FilteredPriorities)
	next.FilteredTypes = maps.Clone(m.cfg.FilteredTypes)
	states, priorities := m.filterPanel.sections[0], m.filterPanel.sections[1]
	stateValues := selectedValues(states.Options, states.Selected)
	priorityValues := selectedValues(priorities.Options, priorities.Selected)
	// Applying one section must not turn the other inherited selections into
	// fixed project overrides. Compare sets because option order is incidental.
	if !maps.Equal(selection(stateValues), selection(m.cfg.GetFilteredStates(code))) {
		next.SetFilteredStates(code, stateValues)
	}
	if !maps.Equal(selection(priorityValues), selection(m.cfg.GetFilteredPriorities(code))) {
		next.SetFilteredPriorities(code, priorityValues)
	}
	typesChanged := false
	if len(m.filterPanel.sections) == 3 {
		types := m.filterPanel.sections[2]
		values := normalizedTypes(selectedValues(types.Options, types.Selected))
		if len(values) > 0 {
			if !m.typeInfo.Found || m.typeInfoErr != nil {
				m.filterError = "Type options unavailable. Choose All types or press Esc, then r to retry."
				return nil
			}
			if _, err := ytcli.IssueTypeQuery(m.typeInfo.FieldName, values); err != nil {
				m.filterError = err.Error()
				return nil
			}
		}
		typesChanged = !slices.Equal(values, m.selectedTypes)
		if typesChanged {
			aliases := m.projectAliases()
			canonical := m.projectCode
			for _, alias := range aliases {
				if alias != "" {
					canonical = alias
					break
				}
			}
			for _, alias := range aliases {
				if alias != canonical {
					delete(next.FilteredTypes, alias)
				}
			}
			next.SetFilteredTypes(canonical, values)
		}
	}
	if err := config.SaveConfig(&next); err != nil {
		m.filterError = "Could not save filters: " + err.Error()
		return nil
	}
	*m.cfg = next
	m.filterMode, m.filterError = false, ""
	if typesChanged || m.err != nil {
		return m.startContext(false)
	}
	m.updateTableRows()
	m.restoreCursor()
	return nil
}

func (m issuesModel) filterNotice() string {
	if m.projectCode == "" {
		return "Type filtering is available in project issue lists."
	}
	if m.typeInfoErr != nil {
		return "Type options unavailable; only saved/observed values shown. [r] Retry after closing filters."
	}
	if m.loading && !m.typeInfo.Found {
		return "Loading project type options..."
	}
	if !m.typeInfo.Found {
		return "No type field found. Custom field names can be set with type_field_name."
	}
	return ""
}

func (m issuesModel) filterPanelView() string {
	width, height := max(1, m.width), max(1, m.height)
	notice := m.filterNotice()
	if m.filterError != "" {
		notice = m.filterError
	}
	reserved := 4
	if notice != "" {
		reserved++
	}
	panelWidth, panelHeight := width, max(1, height-reserved)
	framed := width >= 30 && height >= 10
	if framed {
		panelWidth -= 4
		panelHeight = max(1, panelHeight-2)
	}
	panel := m.filterPanel.View(panelWidth, panelHeight)
	if framed {
		panel = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(ColorViolet)).Padding(0, 1).Render(panel)
	}
	lines := []string{StyleTitle.Render("Issue filters"), panel}
	if notice != "" {
		lines = append(lines, ansi.Truncate(notice, width, "…"))
	}
	lines = append(lines, "←→/Tab Sections · ↑↓ Move · Space Toggle", "Enter Save · Esc Cancel")
	var bounded []string
	for _, line := range strings.Split(strings.Join(lines, "\n"), "\n") {
		bounded = append(bounded, ansi.Truncate(line, width, "…"))
	}
	if len(bounded) > height {
		bounded = bounded[:height]
	}
	return strings.Join(bounded, "\n")
}

func (m issuesModel) filterSummary() string {
	var parts []string
	if m.projectCode != "" {
		value := "All"
		if len(m.selectedTypes) > 0 {
			value = strings.Join(m.selectedTypes, ", ")
		}
		parts = append(parts, "Types: "+value)
	}
	if m.cfg != nil {
		code := m.effectiveProjectCode()
		for _, section := range []struct {
			name              string
			options, selected []string
		}{
			{"States", m.getAvailableStates(), m.cfg.GetFilteredStates(code)},
			{"Priorities", m.getAvailablePriorities(), m.cfg.GetFilteredPriorities(code)},
		} {
			all := true
			for _, option := range section.options {
				if !slices.ContainsFunc(section.selected, func(s string) bool { return strings.EqualFold(s, option) }) {
					all = false
				}
			}
			if !all {
				value := strings.Join(section.selected, ", ")
				if value == "" {
					value = "None"
				}
				parts = append(parts, section.name+": "+value)
			}
		}
	}
	if phrase := m.searchInput.Value(); phrase != "" {
		parts = append(parts, "Text: "+phrase)
	}
	if m.typeInfoErr != nil {
		parts = append(parts, "Type options unavailable (r to retry)")
	}
	return ansi.Truncate(strings.Join(parts, " · "), max(1, m.width), "…")
}
