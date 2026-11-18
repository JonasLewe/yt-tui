package ui

import (
	"fmt"
	"slices"
	"strings"
	"yt-tui/internal/ytcli"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type issueFormTags struct {
	form         int
	tags         []ytcli.Tag
	tagsLoaded   bool
	tagsErr      error
	tagIDs       []string
	tagIndex     int
	tagSearch    textinput.Model
	tagSearching bool
}

func newIssueFormTags(form int) issueFormTags {
	search := textinput.New()
	search.Placeholder = "Find tags by name or owner"
	return issueFormTags{form: form, tagSearch: search}
}

type createTagsLoadedMsg struct {
	form int
	tags []ytcli.Tag
	err  error
}

func (m formModel) loadCreateTags() tea.Cmd {
	// Snapshot the connection before starting the asynchronous request.
	client := m.client
	if client != nil {
		snapshot := *client
		client = &snapshot
	}
	form := m.createTags.form
	return func() tea.Msg {
		tags, err := client.ListTags()
		return createTagsLoadedMsg{form: form, tags: tags, err: err}
	}
}

func tagChoiceLabel(tag ytcli.Tag) string {
	owner := tag.ID
	if tag.Owner != nil && tag.Owner.Login != "" {
		owner = tag.Owner.Login
	}
	return fmt.Sprintf("%s (%s)", safeTagLabel(tag.Name), safeTagLabel(owner))
}

func (f issueFormTags) matchingTags() []ytcli.Tag {
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

func (f issueFormTags) renderTags(focused bool) string {
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

func (m *formModel) updateCreateTagKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.isEdit || m.isClone || m.focusIndex != fieldTags || m.errPopupShow || m.loading {
		return false, nil
	}
	f := &m.createTags
	key := msg.String()
	if f.tagSearching {
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
	switch key {
	case "/":
		f.tagSearching = true
		return true, f.tagSearch.Focus()
	case "enter":
		if f.tagsErr != nil && f.tagsLoaded {
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
	return false, nil
}
