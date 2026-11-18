package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// The configured columns keep their order and preferred widths. Summary gets
// flexible space; metadata remains readable in a typical terminal-sized float.
func issueColumnWidths(fields []issueField, width int) []int {
	widths := make([]int, len(fields))
	if width <= 0 {
		for i, field := range fields {
			widths[i] = field.width
		}
		return widths
	}
	budget := max(0, width-2*len(fields)) // One padding cell on either side.
	minimums := make([]int, len(fields))
	minimumTotal, summary := 0, -1
	for i, field := range fields {
		minimum := max(6, ansi.StringWidth(field.title))
		switch strings.ToLower(field.title) {
		case "id", "type":
			minimum = 8
		case "summary":
			minimum, summary = 16, i
		}
		minimums[i] = min(field.width, minimum)
		minimumTotal += minimums[i]
	}
	if budget < minimumTotal {
		// Very small windows still show every configured column in the original
		// order. Prefer readable labels over horizontal overflow.
		for i := range widths {
			if minimumTotal > 0 {
				widths[i] = budget * minimums[i] / minimumTotal
			}
		}
		remaining := budget
		for _, value := range widths {
			remaining -= value
		}
		for i := 0; remaining > 0 && len(widths) > 0; i = (i + 1) % len(widths) {
			widths[i]++
			remaining--
		}
		return widths
	}
	copy(widths, minimums)
	remaining := budget - minimumTotal
	if summary >= 0 {
		grow := min(remaining, fields[summary].width-widths[summary])
		widths[summary] += grow
		remaining -= grow
	}
	for remaining > 0 {
		changed := false
		for i, field := range fields {
			if remaining > 0 && widths[i] < field.width {
				widths[i]++
				remaining--
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	if summary >= 0 {
		widths[summary] += remaining
	}
	return widths
}

func (m issuesModel) issueHelp() string {
	if m.width < 70 {
		return StyleHelp.Render("[F] Filter  [/] Search\n[Enter] Detail  [Esc] Back")
	}
	if m.width < 145 {
		return StyleHelp.Render("[F] Filter  [/] Search  [Enter] Detail  [Esc] Back\n[↑↓] Move  [Space] Action  [?] Help  " + quitHelp())
	}
	help := "[Esc] Back  [↑↓] Move  [Space] Action  [Enter] Detail  [/] Search  [F] Filter  [s] Sort  [n] New  [r] Refresh  [?] Help  " + quitHelp()
	if m.projectCode != "" && m.projectCode != "ME" {
		help = "[K] Knowledge Base  " + help
	}
	return StyleHelp.Render(help)
}

// Cell-aware clipping is a final guard for long project names, error messages,
// and overlays. It preserves ANSI styles and prevents the root padding from
// wrapping a child screen into additional terminal rows.
func boundIssueView(view string, width, height int) string {
	if width <= 0 || height <= 0 {
		return view
	}
	lines := strings.Split(view, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "…")
	}
	return strings.Join(lines, "\n")
}
