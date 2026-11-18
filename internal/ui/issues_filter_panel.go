package ui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type issueFilterSection struct {
	Title      string
	Options    []string
	Selected   map[string]bool
	AllowAll   bool
	AllLabel   string
	Searchable bool
	Search     string
}

type issuesFilterPanel struct {
	sections    []issueFilterSection
	section     int
	cursor      int
	searchMode  bool
	searchInput textinput.Model
}

// Keep draft selections separate from the saved configuration until Enter.
func newIssuesFilterPanel(sections []issueFilterSection) issuesFilterPanel {
	p := issuesFilterPanel{sections: make([]issueFilterSection, len(sections))}
	for i, section := range sections {
		p.sections[i] = section
		p.sections[i].Options = append([]string(nil), section.Options...)
		p.sections[i].Selected = make(map[string]bool, len(section.Selected))
		for option, selected := range section.Selected {
			p.sections[i].Selected[option] = selected
		}
	}
	p.clamp()
	return p
}

func (s issueFilterSection) visibleOptions() []string {
	if s.Search == "" {
		return s.Options
	}
	var options []string
	for _, option := range s.Options {
		if strings.Contains(strings.ToLower(option), strings.ToLower(s.Search)) {
			options = append(options, option)
		}
	}
	return options
}

func (p *issuesFilterPanel) startSearch() tea.Cmd {
	if len(p.sections) == 0 || !p.sections[p.section].Searchable {
		return nil
	}
	p.searchInput = textinput.New()
	p.searchInput.SetValue(p.sections[p.section].Search)
	p.searchInput.CharLimit = 200
	p.searchMode = true
	return p.searchInput.Focus()
}

func (p *issuesFilterPanel) updateSearch(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		p.sections[p.section].Search = ""
		p.cursor = 0
		fallthrough
	case "enter":
		p.searchMode = false
		p.searchInput.Blur()
		return nil
	case "tab", "shift+tab":
		p.searchMode = false
		p.searchInput.Blur()
		p.move(msg.String())
		return nil
	}
	var cmd tea.Cmd
	p.searchInput, cmd = p.searchInput.Update(msg)
	p.sections[p.section].Search = p.searchInput.Value()
	p.cursor = 0
	return cmd
}

func (s issueFilterSection) rows() int {
	if s.AllowAll {
		return len(s.visibleOptions()) + 1
	}
	return len(s.visibleOptions())
}

func (s issueFilterSection) allSelected() bool {
	for _, selected := range s.Selected {
		if selected {
			return false
		}
	}
	return true
}

func (p *issuesFilterPanel) clamp() {
	if len(p.sections) == 0 {
		p.section, p.cursor = 0, 0
		return
	}
	p.section = max(0, min(p.section, len(p.sections)-1))
	if p.sections[p.section].rows() == 0 {
		for i, section := range p.sections {
			if section.rows() > 0 {
				p.section = i
				break
			}
		}
	}
	p.cursor = max(0, min(p.cursor, p.sections[p.section].rows()-1))
}

func (p *issuesFilterPanel) move(key string) {
	p.clamp()
	if len(p.sections) == 0 {
		return
	}
	rows := p.sections[p.section].rows()
	switch key {
	case "up", "k":
		if rows > 0 {
			p.cursor = (p.cursor + rows - 1) % rows
		}
	case "down", "j":
		if rows > 0 {
			p.cursor = (p.cursor + 1) % rows
		}
	case "left", "h", "shift+tab", "right", "l", "tab":
		direction := 1
		if key == "left" || key == "h" || key == "shift+tab" {
			direction = -1
		}
		for step := 1; step <= len(p.sections); step++ {
			next := (p.section + direction*step + len(p.sections)) % len(p.sections)
			if p.sections[next].rows() > 0 {
				p.section = next
				break
			}
		}
		p.clamp()
	}
}

func (p *issuesFilterPanel) toggle() {
	p.clamp()
	if len(p.sections) == 0 {
		return
	}
	section := &p.sections[p.section]
	if section.rows() == 0 {
		return
	}
	if section.AllowAll && p.cursor == 0 {
		clear(section.Selected)
		return
	}
	index := p.cursor
	if section.AllowAll {
		index--
	}
	option := section.visibleOptions()[index]
	if section.Selected[option] {
		delete(section.Selected, option)
	} else {
		if section.Selected == nil {
			section.Selected = make(map[string]bool)
		}
		section.Selected[option] = true
	}
}

// View includes section headings and choices. The containing issues view owns
// the dialog title, border, and shared keyboard help.
func (p issuesFilterPanel) View(width, height int) string {
	if width <= 0 || height <= 0 || len(p.sections) == 0 {
		return ""
	}
	p.clamp()
	const gap = 2
	narrow := width < len(p.sections)*22+(len(p.sections)-1)*gap
	if narrow {
		heading := fmt.Sprintf("%s (%d/%d)", p.sections[p.section].Title, p.section+1, len(p.sections))
		return p.sectionView(p.section, heading, width, height)
	}
	columnWidth := (width - (len(p.sections)-1)*gap) / len(p.sections)
	columns := make([]string, 0, len(p.sections))
	for i, section := range p.sections {
		column := p.sectionView(i, section.Title, columnWidth, height)
		if i+1 < len(p.sections) {
			column = lipgloss.NewStyle().Width(columnWidth).MarginRight(gap).Render(column)
		}
		columns = append(columns, column)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, columns...)
}

func (p issuesFilterPanel) sectionView(index int, heading string, width, height int) string {
	section := p.sections[index]
	active := index == p.section
	rows := section.rows()
	visibleRows := max(0, height-1)
	if section.Searchable {
		visibleRows = max(0, visibleRows-1)
	}
	start := 0
	if active && visibleRows > 0 && p.cursor >= visibleRows {
		start = p.cursor - visibleRows + 1
	}
	start = min(start, max(0, rows-visibleRows))
	end := min(rows, start+visibleRows)
	if start > 0 {
		heading += " ↑"
	}
	if end < rows {
		heading += " ↓"
	}
	color := ColorViolet
	if active {
		color = ColorCyan
	}
	lines := []string{lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true).
		Render(filterPanelLine(heading, width))}
	if section.Searchable && height > 1 {
		label := "[/] Search tags"
		if section.Search != "" || (active && p.searchMode) {
			label = "/ " + section.Search
		}
		if active && p.searchMode {
			input := p.searchInput
			input.Prompt = "/ "
			input.Width = max(1, width-2)
			label = input.View()
		}
		if active && p.searchMode {
			lines = append(lines, ansi.Truncate(label, width, ""))
		} else {
			lines = append(lines, StyleSubtext.Render(filterPanelLine(label, width)))
		}
	}
	if rows == 0 && visibleRows > 0 {
		lines = append(lines, StyleSubtext.Render(filterPanelLine("No options", width)))
	}
	for row := start; row < end; row++ {
		var label string
		var selected bool
		if section.AllowAll && row == 0 {
			label, selected = section.AllLabel, section.allSelected()
			if label == "" {
				label = "All types"
			}
		} else {
			optionIndex := row
			if section.AllowAll {
				optionIndex--
			}
			label = section.visibleOptions()[optionIndex]
			selected = section.Selected[label]
		}
		check := "[ ]"
		if selected {
			check = "[x]"
		}
		prefix := "  "
		focused := active && row == p.cursor
		if focused {
			prefix = "> "
		}
		line := filterPanelLine(prefix+check+" "+label, width)
		if focused {
			line = StyleSelected.Render(line)
		}
		lines = append(lines, line)
	}
	if section.Searchable && len(section.visibleOptions()) == 0 && len(lines) < height {
		label := "No visible tags"
		if section.Search != "" {
			label = "No matching tags"
		}
		lines = append(lines, StyleSubtext.Render(filterPanelLine(label, width)))
	}
	return strings.Join(lines, "\n")
}

func filterPanelLine(value string, width int) string {
	value = safeTagLabel(value)
	return ansi.Truncate(value, width, "…")
}

// API-controlled names are plain labels, never terminal escape sequences.
func safeTagLabel(value string) string {
	value = ansi.Strip(value)
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
}
