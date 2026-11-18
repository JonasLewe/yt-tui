package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"yt-tui/internal/ytcli"
)

type newIssueFields struct {
	project      string
	version      int
	meta         ytcli.IssueCreateFields
	metaReady    bool
	metaErr      error
	stateIndex   int // zero leaves the initial state to the project workflow
	tags         []ytcli.Tag
	tagsLoaded   bool
	tagsErr      error
	tagIDs       []string
	tagIndex     int
	tagSearch    textinput.Model
	tagSearching bool
}

type createFieldsLoadedMsg struct {
	form    int
	version int
	project string
	fields  ytcli.IssueCreateFields
	err     error
}

type createTagsLoadedMsg struct {
	form int
	tags []ytcli.Tag
	err  error
}

func (m formModel) loadCreateTags() tea.Cmd {
	return func() tea.Msg {
		tags, err := m.client.ListTags()
		return createTagsLoadedMsg{form: m.formNumber, tags: tags, err: err}
	}
}

func (m *formModel) refreshProjectFields() tea.Cmd {
	code := m.getSelectedProjectCode()
	if code == "" || code == m.createFields.project {
		return nil
	}
	// Type and priority choices come from project-specific configurations.
	if !m.isEdit && !m.isClone && m.createFields.project != "" {
		m.typeIndex, m.priorityIndex = 0, 0
	}
	if m.typeIndex >= len(m.getTypes()) {
		m.typeIndex = 0
	}
	if m.priorityIndex >= len(m.getPriorities()) {
		m.priorityIndex = 0
	}
	m.allUsers, m.filteredUsers = nil, nil
	m.assigneeInput.Placeholder = "Unassigned"
	m.createFields.project = code
	m.createFields.version++
	if m.isClone || m.isEdit || len(m.projects) == 0 {
		return m.loadUsersCmd(code)
	}
	m.createFields.meta = ytcli.IssueCreateFields{}
	m.createFields.stateIndex, m.createFields.metaReady, m.createFields.metaErr = 0, false, nil
	project := m.projects[m.projectIndex]
	client, form, version := m.client, m.formNumber, m.createFields.version
	return tea.Batch(m.loadUsersCmd(code), func() tea.Msg {
		fields, err := client.GetIssueCreateFields(project.ID)
		return createFieldsLoadedMsg{form: form, version: version, project: code, fields: fields, err: err}
	})
}

func (m formModel) createIssue(project, summary, priority, issueType string) (string, error) {
	if m.isClone {
		return m.client.CreateIssue(project, summary, m.descTextArea.Value(), priority, issueType, m.assigneeInput.Value())
	}
	f := m.createFields
	// The user can submit while project metadata is loading. Resolve the
	// assignment field first so a quick save has the same defaults.
	if !f.metaReady || f.project != project {
		p, err := m.client.ResolveIssueTypeProject(project)
		if err != nil {
			return "", err
		}
		f.meta, _ = m.client.GetIssueCreateFields(p.ID)
		f.stateIndex = 0
	}
	opts := ytcli.CreateIssueOptions{Fields: &f.meta, TagIDs: f.tagIDs}
	if f.stateIndex > 0 && f.stateIndex <= len(f.meta.States) {
		opts.StateID = f.meta.States[f.stateIndex-1].ID
	}
	return m.client.CreateIssueWithOptions(project, summary, m.descTextArea.Value(), priority, issueType, m.assigneeInput.Value(), opts)
}

func tagChoiceLabel(tag ytcli.Tag) string {
	owner := tag.ID
	if tag.Owner != nil && tag.Owner.Login != "" {
		owner = tag.Owner.Login
	}
	return fmt.Sprintf("%s (%s)", safeTagLabel(tag.Name), safeTagLabel(owner))
}

func (f newIssueFields) matchingTags() []ytcli.Tag {
	query := strings.ToLower(strings.TrimSpace(f.tagSearch.Value()))
	if query == "" {
		return f.tags
	}
	var matches []ytcli.Tag
	for _, tag := range f.tags {
		if strings.Contains(strings.ToLower(tagChoiceLabel(tag)), query) {
			matches = append(matches, tag)
		}
	}
	return matches
}

func (f newIssueFields) renderState() string {
	switch {
	case !f.metaReady:
		return "Project default · loading choices…"
	case f.metaErr != nil:
		return "Project default · " + f.metaErr.Error()
	case f.stateIndex > 0:
		return "◀  " + f.meta.States[f.stateIndex-1].Name + "  ▶"
	case len(f.meta.States) > 0:
		return "◀  Project default  ▶"
	default:
		return "Project default"
	}
}

func (f newIssueFields) renderTags(focused bool) string {
	if !f.tagsLoaded {
		return "Loading tags…"
	}
	if f.tagsErr != nil {
		return f.tagsErr.Error() + " · Enter to retry"
	}
	var selected []string
	for _, tag := range f.tags {
		if slices.Contains(f.tagIDs, tag.ID) {
			selected = append(selected, tagChoiceLabel(tag))
		}
	}
	label := strings.Join(selected, ", ")
	if label == "" {
		label = "None"
	}
	if !focused {
		return label
	}
	if f.tagSearching || f.tagSearch.Value() != "" {
		label += "\n" + f.tagSearch.View()
	}
	matches := f.matchingTags()
	if len(matches) == 0 {
		return label + "\nNo matching tags"
	}
	tag := matches[f.tagIndex]
	check := "[ ]"
	if slices.Contains(f.tagIDs, tag.ID) {
		check = "[x]"
	}
	return fmt.Sprintf("%s\n◀ %s %s ▶  %d/%d", label, check, tagChoiceLabel(tag), f.tagIndex+1, len(matches))
}

// updateCreateFieldKey consumes keys owned by the two create-only selectors.
func (m *formModel) updateCreateFieldKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.isEdit || m.isClone {
		return false, nil
	}
	f := &m.createFields
	key := msg.String()
	if m.focusIndex == fieldTags && f.tagSearching {
		switch key {
		case "esc":
			f.tagSearch.SetValue("")
			f.tagIndex = 0
			fallthrough
		case "enter":
			f.tagSearching = false
			f.tagSearch.Blur()
			return true, nil
		default:
			var cmd tea.Cmd
			f.tagSearch, cmd = f.tagSearch.Update(msg)
			f.tagIndex = 0
			return true, cmd
		}
	}
	if m.focusIndex == fieldTags {
		switch key {
		case "/":
			f.tagSearching = true
			return true, f.tagSearch.Focus()
		case "enter":
			if f.tagsErr != nil {
				f.tagsLoaded = false
				return true, m.loadCreateTags()
			}
		}
		matches := f.matchingTags()
		if len(matches) > 0 {
			switch key {
			case "left", "h":
				f.tagIndex = (f.tagIndex + len(matches) - 1) % len(matches)
				return true, nil
			case "right", "l":
				f.tagIndex = (f.tagIndex + 1) % len(matches)
				return true, nil
			case " ", "enter":
				id := matches[f.tagIndex].ID
				if i := slices.Index(f.tagIDs, id); i >= 0 {
					f.tagIDs = slices.Delete(f.tagIDs, i, i+1)
				} else {
					f.tagIDs = append(f.tagIDs, id)
				}
				return true, nil
			}
		}
	}
	if m.focusIndex == fieldState {
		count := len(f.meta.States) + 1
		switch key {
		case "left", "h":
			f.stateIndex = (f.stateIndex + count - 1) % count
			return true, nil
		case "right", "l":
			f.stateIndex = (f.stateIndex + 1) % count
			return true, nil
		case "enter":
			if f.metaErr != nil {
				f.project = ""
				cmd := m.refreshProjectFields()
				return true, cmd
			}
		}
	}
	return false, nil
}
