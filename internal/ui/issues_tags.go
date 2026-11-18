package ui

import (
	"fmt"
	"slices"
	"strings"
	"yt-tui/internal/ytcli"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Tag names are YouTrack's documented search identity. Owners/IDs cannot be
// encoded in an issue search, so identical visible names are a single choice.
func normalizedTags(names []string) []string {
	// YouTrack tag-name search is case-insensitive. Preserve the first spelling
	// (saved selections first) so one query value remains one checked option.
	return normalizedTypes(names)
}

func tagNames(tags []ytcli.Tag) []string {
	var names []string
	for _, tag := range tags {
		if tag.Name != "" {
			names = append(names, tag.Name)
		}
	}
	return normalizedTags(names)
}

func detailTags(issue ytcli.Issue, width int) string {
	names := tagNames(issue.Tags)
	label := "None"
	if len(names) > 0 {
		label = strings.Join(names, ", ")
	}
	// Tags remain scrollable with the description even for large tag collections.
	label = safeTagLabel(label)
	return lipgloss.NewStyle().Foreground(lipgloss.Color(ColorCyan)).Render(ansi.Hardwrap("Tags: "+label, max(1, width), false))
}

type issueTagsMsg struct {
	contextKey string
	requestID  uint64
	metadataID uint64
	scope      string
	scopeErr   error
	withTags   bool
	tags       []ytcli.Tag
	err        error
}

func (m issuesModel) clientContext() string {
	if m.client == nil {
		return ""
	}
	return m.client.ContextKey()
}

func (m *issuesModel) startContext(refresh bool) tea.Cmd {
	// A new generation rejects previous in-flight catalogue requests.
	m.tagsLoading = false
	context := m.clientContext()
	if m.contextKey != context {
		m.contextKey = context
		m.tagScope, m.tagScopeErr = "", nil
		m.tagScopeReady, m.tagScopeLoading = false, false
		m.tags, m.tagsErr = nil, nil
		m.tagsLoaded, m.tagsLoading = false, false
	}
	if refresh {
		m.tags, m.tagsErr = nil, nil
		m.tagsLoaded, m.tagsLoading = false, false
		if m.tagScopeErr != nil {
			m.tagScopeErr = nil
		}
	}
	needsScope := m.projectCode != "" && m.cfg != nil && len(m.cfg.FilteredTags) > 0 && !m.tagScopeReady
	m.tagScopeLoading = needsScope
	cmd := m.startIssueContext(refresh)
	if needsScope {
		scopeCmd := m.loadTagsCmd(false)
		if cmd == nil {
			return scopeCmd
		}
		return tea.Batch(cmd, scopeCmd)
	}
	return cmd
}

func (m *issuesModel) loadTagsCmd(withTags bool) tea.Cmd {
	m.tagsRequestID++
	msg := issueTagsMsg{contextKey: m.contextKey, requestID: m.requestID, metadataID: m.tagsRequestID, scope: m.tagScope, withTags: withTags}
	m.tagsLoading = withTags
	if !m.tagScopeReady {
		m.tagScopeLoading = true
	}
	if m.client == nil {
		return func() tea.Msg { msg.scopeErr = fmt.Errorf("YouTrack client is unavailable"); return msg }
	}
	client := *m.client
	ready := m.tagScopeReady
	return func() tea.Msg {
		if !ready {
			msg.scope, msg.scopeErr = client.TagFilterScope()
		}
		if msg.scopeErr == nil && withTags {
			msg.tags, msg.err = client.ListTags()
		}
		return msg
	}
}

func (m issuesModel) availableTags() []string {
	names := append([]string(nil), m.selectedTags...)
	if m.filterMode && len(m.filterPanel.sections) > 3 {
		section := m.filterPanel.sections[3]
		names = append(names, selectedValues(section.Options, section.Selected)...)
	}
	names = append(names, tagNames(m.tags)...)
	return normalizedTags(names)
}

func (m issuesModel) unresolvedSavedTagFilter() bool {
	if m.projectCode == "" || m.cfg == nil || !m.tagScopeReady || m.typeInfoErr == nil || m.typeInfo.Project.ID != "" {
		return false
	}
	entries := m.cfg.FilteredTags[m.tagScope]
	if _, explicit := entries[m.projectCode]; explicit {
		return false
	}
	for code, names := range entries {
		if code != "default" && len(names) > 0 {
			return true
		}
	}
	return false
}

func (m *issuesModel) applyTagQuery() bool {
	if m.projectCode == "" {
		return true
	}
	if m.tagScopeErr != nil && m.cfg != nil && len(m.cfg.FilteredTags) > 0 {
		m.err = fmt.Errorf("cannot restore tag filters for this account: %w; press r to retry", m.tagScopeErr)
		m.loading = false
		return false
	}
	if m.unresolvedSavedTagFilter() {
		m.err = fmt.Errorf("cannot resolve saved project tag filters: %w; choose No restriction or retry", m.typeInfoErr)
		m.loading = false
		return false
	}
	previous := m.selectedTags
	if m.tagScopeReady {
		m.selectedTags = normalizedTags(m.cfg.GetFilteredTags(m.tagScope, m.projectAliases()...))
	}
	if m.filterMode && len(m.filterPanel.sections) > 3 {
		section := &m.filterPanel.sections[3]
		if slices.Equal(normalizedTags(selectedValues(section.Options, section.Selected)), previous) {
			section.Selected = selection(m.selectedTags)
		}
		section.Options = m.availableTags()
	}
	if len(m.selectedTags) == 0 {
		return true
	}
	clause, err := ytcli.TagQuery(m.selectedTags)
	if err != nil {
		m.err, m.loading = err, false
		return false
	}
	m.serverQuery = andIssueQuery(m.serverQuery, clause)
	return true
}

func andIssueQuery(base, clause string) string {
	if base == "" {
		return clause
	}
	if clause == "" {
		return base
	}
	return "(" + base + ") and (" + clause + ")"
}

func (m issuesModel) tagNotice() string {
	if m.projectCode == "" {
		return ""
	}
	if m.tagScopeErr != nil {
		return "Tag account unavailable; saved filters retained. Close filters and press r to retry."
	}
	if m.tagsLoading || m.tagScopeLoading {
		return "Loading visible tags..."
	}
	if m.tagsErr != nil {
		return "Tag options unavailable; saved selections retained. Close filters and press r to retry."
	}
	if m.tagsLoaded {
		available := tagNames(m.tags)
		for _, name := range m.selectedTags {
			if !slices.ContainsFunc(available, func(value string) bool { return strings.EqualFold(value, name) }) {
				return "Some saved tags are unavailable (renamed, deleted or private); selection still applies."
			}
		}
	}
	return "Tags match any selected name; No restriction also includes issues without tags."
}
