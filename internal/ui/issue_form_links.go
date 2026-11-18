package ui

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"yt-tui/internal/ytcli"
)

type formIssueLink struct {
	RelationID, Label        string
	TargetID, TargetReadable string
}

type linkPickerMode int

const (
	linksClosed linkPickerMode = iota
	linksSelected
	linksRelations
	linksTarget
)

type formIssueLinks struct {
	selected  []formIssueLink
	relations []formIssueLink
	mode      linkPickerMode
	cursor    int
	request   uint64
	loading   bool
	message   string
	pending   formIssueLink
	search    textinput.Model
	results   []ytcli.Issue
}

type formLinkTypesMsg struct {
	request uint64
	types   []ytcli.IssueLinkType
	err     error
}

type formLinkTargetsMsg struct {
	request uint64
	issues  []ytcli.Issue
	err     error
}

func (l formIssueLinks) open() bool { return l.mode != linksClosed }

func (m *formModel) setupIssueLinks(data string) (string, tea.Cmd) {
	m.links = formIssueLinks{request: m.links.request + 1}
	parts := strings.SplitN(data, ":", 4)
	if len(parts) != 4 || parts[0] != "linked" {
		return data, nil
	}
	m.links.pending = formIssueLink{TargetID: parts[2], TargetReadable: parts[3]}
	return parts[1], m.startLinkRelations()
}

func (m *formModel) startLinkRelations() tea.Cmd {
	l := &m.links
	l.mode, l.cursor, l.message = linksRelations, 0, ""
	if len(l.relations) > 0 {
		return nil
	}
	l.request++
	l.loading = true
	client, request := m.client, l.request
	return func() tea.Msg {
		types, err := client.ListIssueLinkTypes()
		return formLinkTypesMsg{request: request, types: types, err: err}
	}
}

// Relation labels always describe the new issue from its own perspective.
func issueLinkChoices(types []ytcli.IssueLinkType) []formIssueLink {
	var choices []formIssueLink
	for _, t := range types {
		if t.ID == "" {
			continue
		}
		name := cmp.Or(t.LocalizedName, t.Name, t.ID)
		outward := cmp.Or(t.LocalizedSourceToTarget, t.SourceToTarget)
		if t.Directed {
			choices = append(choices,
				formIssueLink{RelationID: t.ID + "s", Label: cmp.Or(outward, name+" (outward)")},
				formIssueLink{RelationID: t.ID + "t", Label: cmp.Or(t.LocalizedTargetToSource, t.TargetToSource, name+" (inward)")})
		} else {
			choices = append(choices, formIssueLink{RelationID: t.ID, Label: cmp.Or(outward, name)})
		}
	}
	return choices
}

func (l *formIssueLinks) addPending() {
	exists := slices.ContainsFunc(l.selected, func(link formIssueLink) bool {
		return link.TargetID == l.pending.TargetID && link.RelationID == l.pending.RelationID
	})
	if !exists {
		l.selected = append(l.selected, l.pending)
	}
	l.close()
}

func (l *formIssueLinks) close() {
	l.request++
	l.mode, l.loading, l.message, l.cursor = linksClosed, false, "", 0
	l.pending = formIssueLink{}
	l.results = nil
	l.search.Blur()
}

func (l formIssueLinks) summary() string {
	if len(l.selected) == 0 {
		return "Add link… [Enter]"
	}
	var labels []string
	for _, link := range l.selected {
		labels = append(labels, link.Label+" "+link.TargetReadable)
	}
	return strings.Join(labels, ", ") + "  [Enter]"
}

func (m *formModel) updateIssueLinks(msg tea.Msg) (bool, tea.Cmd) {
	l := &m.links
	switch msg := msg.(type) {
	case formLinkTypesMsg:
		if msg.request == l.request && l.mode == linksRelations {
			l.loading = false
			l.relations = issueLinkChoices(msg.types)
			if msg.err != nil {
				l.message = msg.err.Error()
			} else if len(l.relations) == 0 {
				l.message = "No link types available"
			}
		}
	case formLinkTargetsMsg:
		if msg.request == l.request && l.mode == linksTarget {
			l.results, l.loading, l.cursor = msg.issues, false, 0
			if msg.err != nil {
				l.message = msg.err.Error()
			} else if len(l.results) == 0 {
				l.message = "No matching issues"
			}
		}
	case tea.KeyMsg:
		if m.loading || m.pendingSave != nil || m.errPopupShow || m.filepickerActive || m.isEdit || m.isClone {
			return false, nil
		}
		key := msg.String()
		if !l.open() {
			if m.focusIndex != fieldLinks || key != "enter" {
				return false, nil
			}
			if len(l.selected) == 0 {
				return true, m.startLinkRelations()
			}
			l.mode, l.cursor = linksSelected, 0
		} else if key == "esc" {
			fromDetail := l.pending.TargetID != ""
			l.close()
			if fromDetail {
				return true, func() tea.Msg { return popStateMsg{} }
			}
		} else {
			return true, m.handleLinkPickerKey(msg)
		}
	default:
		return false, nil
	}
	return true, nil
}

var linkTargetID = regexp.MustCompile(`(?i)^[a-z0-9_][a-z0-9_-]*-[0-9]+$`)

func (m *formModel) handleLinkPickerKey(msg tea.KeyMsg) tea.Cmd {
	l := &m.links
	key := msg.String()
	var count int
	switch l.mode {
	case linksSelected:
		count = len(l.selected) + 1 // first row adds another link
		if key == "enter" && l.cursor == 0 {
			return m.startLinkRelations()
		}
		if (key == "d" || key == "delete") && l.cursor > 0 {
			l.selected = slices.Delete(l.selected, l.cursor-1, l.cursor)
			l.cursor = min(l.cursor, len(l.selected))
			return nil
		}
	case linksRelations:
		count = len(l.relations)
		if key == "enter" && !l.loading {
			if count == 0 {
				return m.startLinkRelations()
			}
			choice := l.relations[l.cursor]
			l.pending.RelationID, l.pending.Label = choice.RelationID, choice.Label
			if l.pending.TargetID != "" {
				l.addPending()
				return nil
			}
			l.mode, l.cursor, l.message = linksTarget, 0, ""
			l.search = textinput.New()
			l.search.Placeholder = "Issue ID or YouTrack search"
			return l.search.Focus()
		}
	case linksTarget:
		count = len(l.results)
		switch key {
		case "enter":
			if count > 0 {
				issue := l.results[l.cursor]
				l.pending.TargetID, l.pending.TargetReadable = issue.ID, issue.IDReadable
				l.addPending()
				return nil
			}
			query := strings.TrimSpace(l.search.Value())
			if query == "" || l.loading {
				return nil
			}
			if linkTargetID.MatchString(query) {
				query = "issue id: " + query
			}
			l.request++
			request, client := l.request, m.client
			l.loading, l.message = true, ""
			return func() tea.Msg {
				issues, err := client.ListIssues("", query, 50, 0)
				return formLinkTargetsMsg{request: request, issues: issues, err: err}
			}
		case "up", "down":
		default:
			old := l.search.Value()
			var cmd tea.Cmd
			l.search, cmd = l.search.Update(msg)
			if old != l.search.Value() {
				l.results, l.cursor, l.message, l.loading = nil, 0, "", false
				l.request++
			}
			return cmd
		}
	}
	switch key {
	case "up", "k":
		l.cursor = max(0, l.cursor-1)
	case "down", "j":
		l.cursor = min(max(0, count-1), l.cursor+1)
	}
	return nil
}

func (l formIssueLinks) view(width, height int) string {
	w := max(8, min(72, width-6))
	var title, help string
	var rows []string
	switch l.mode {
	case linksSelected:
		title, help = "Issue links", "↑/↓ Move · Enter Add · d Remove · Esc Back"
		rows = append(rows, "Add another link…")
		for _, link := range l.selected {
			rows = append(rows, link.Label+" "+link.TargetReadable)
		}
	case linksRelations:
		title = "New issue → relationship → " + cmp.Or(l.pending.TargetReadable, "existing issue")
		help = "↑/↓ Move · Enter Select · Esc Cancel"
		if len(l.relations) == 0 && !l.loading {
			help = "Enter Retry · Esc Cancel"
		}
		for _, relation := range l.relations {
			rows = append(rows, relation.Label)
		}
	case linksTarget:
		l.search.Width = max(1, w-4)
		title = "New issue " + l.pending.Label + "…\n" + l.search.View()
		help = "Enter Search · Esc Cancel"
		if len(l.results) > 0 {
			help = "↑/↓ Move · Enter Add link · Esc Cancel"
		}
		for _, issue := range l.results {
			rows = append(rows, fmt.Sprintf("%s  %s", issue.IDReadable, issue.Summary))
		}
	}
	body := []string{StyleTitle.Render(title)}
	limit := max(1, min(12, height-8))
	start := max(0, l.cursor-limit+1)
	for i := start; i < min(len(rows), start+limit); i++ {
		text := ansi.Truncate(strings.Join(strings.Fields(rows[i]), " "), w-3, "…")
		if i == l.cursor {
			body = append(body, StyleSelected.Render("▶ "+text))
		} else {
			body = append(body, "  "+text)
		}
	}
	if l.loading {
		body = append(body, "Loading…")
	} else if l.message != "" {
		body = append(body, l.message)
	}
	body = append(body, StyleHelp.Render(help))
	picker := StyleNormalBorder.Width(w).Render(strings.Join(body, "\n"))
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, picker)
}
