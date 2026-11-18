package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestIssuesFilterPanelNavigation(t *testing.T) {
	p := newIssuesFilterPanel([]issueFilterSection{
		{Title: "States", Options: []string{"Open", "Done"}},
		{Title: "Priorities"},
		{Title: "Types", Options: []string{"Epic", "Story"}, AllowAll: true},
	})
	p.move("k")
	if p.section != 0 || p.cursor != 1 {
		t.Fatalf("up should wrap within states: section=%d cursor=%d", p.section, p.cursor)
	}
	p.move("j")
	if p.cursor != 0 {
		t.Fatal("down should wrap to first state")
	}
	p.move("right")
	if p.section != 2 || p.cursor != 0 {
		t.Fatal("right should skip empty priorities and reach All types")
	}
	p.move("up")
	if p.cursor != 2 {
		t.Fatal("up should wrap from All types to Story")
	}
	p.move("h")
	if p.section != 0 || p.cursor != 1 {
		t.Fatal("left should skip empty section and clamp row to last state")
	}
	p.move("shift+tab")
	if p.section != 2 {
		t.Fatal("backward section navigation should wrap")
	}
	p.move("tab")
	if p.section != 0 {
		t.Fatal("forward section navigation should wrap")
	}
}

func TestIssuesFilterPanelTypeSelectionAndAll(t *testing.T) {
	p := newIssuesFilterPanel([]issueFilterSection{{
		Title: "Types", Options: []string{"Epic", "Story"}, AllowAll: true,
	}})
	if !strings.Contains(ansi.Strip(p.View(40, 10)), "[x] All types") {
		t.Fatal("no explicit selection should show All types selected")
	}
	p.move("down")
	p.toggle()
	p.move("down")
	p.toggle()
	selected := p.sections[0].Selected
	if !selected["Epic"] || !selected["Story"] || p.sections[0].allSelected() {
		t.Fatal("multiple specific types should be selectable without All")
	}
	p.toggle()
	p.move("up")
	p.toggle()
	if !p.sections[0].allSelected() {
		t.Fatal("removing the last explicit type should return to All")
	}
	p.toggle()
	p.move("up")
	p.toggle()
	if len(p.sections[0].Selected) != 0 {
		t.Fatal("selecting All should clear explicit type selections")
	}
}

func TestIssuesFilterPanelDraftIsolationAndEmptyStateSelection(t *testing.T) {
	selected := map[string]bool{"Open": true}
	options := []string{"Open", "Done"}
	p := newIssuesFilterPanel([]issueFilterSection{{Title: "States", Options: options, Selected: selected}})
	p.toggle()
	if len(p.sections[0].Selected) != 0 {
		t.Fatal("states can have no selected options")
	}
	if !selected["Open"] {
		t.Fatal("editing the draft mutated the source selection")
	}
	options[0] = "Changed outside dialog"
	view := ansi.Strip(p.View(50, 10))
	if !strings.Contains(view, "[ ] Open") || strings.Contains(view, "All types") {
		t.Fatalf("state options must be copied and must not gain All semantics: %q", view)
	}
}

func TestIssuesFilterPanelEmptySections(t *testing.T) {
	for _, sections := range [][]issueFilterSection{nil, {{Title: "States"}, {Title: "Priorities"}}} {
		p := newIssuesFilterPanel(sections)
		for _, key := range []string{"up", "down", "left", "right", "h", "j", "k", "l"} {
			p.move(key)
			p.toggle()
		}
		if p.cursor < 0 || p.section < 0 {
			t.Fatal("empty sections must not produce negative positions")
		}
		_ = p.View(20, 4)
	}
	p := newIssuesFilterPanel([]issueFilterSection{{Title: "Types", AllowAll: true}})
	p.toggle()
	if !strings.Contains(ansi.Strip(p.View(20, 4)), "[x] All types") {
		t.Fatal("a type section without options must still allow All types")
	}
}

func TestIssuesFilterPanelResponsiveRendering(t *testing.T) {
	p := newIssuesFilterPanel([]issueFilterSection{
		{Title: "States", Options: []string{"Open", "Done"}},
		{Title: "Priorities", Options: []string{"Normal"}},
		{Title: "Types", Options: []string{"Epic", "Story"}, AllowAll: true},
	})
	wide := ansi.Strip(p.View(90, 8))
	for _, title := range []string{"States", "Priorities", "Types"} {
		if !strings.Contains(wide, title) {
			t.Errorf("wide view should display %s", title)
		}
	}
	p.move("right")
	p.move("right")
	narrow := ansi.Strip(p.View(25, 8))
	if !strings.Contains(narrow, "Types (3/3)") || strings.Contains(narrow, "States") {
		t.Fatalf("narrow view should show only the active section: %q", narrow)
	}
	for _, size := range [][2]int{{90, 8}, {25, 8}, {1, 1}, {8, 2}, {0, 3}, {30, 0}} {
		view := p.View(size[0], size[1])
		if view == "" {
			continue
		}
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Errorf("view exceeds %dx%d: %dx%d", size[0], size[1], lipgloss.Width(view), lipgloss.Height(view))
		}
	}
}

func TestIssuesFilterPanelKeepsLongListCursorVisible(t *testing.T) {
	var options []string
	for i := 0; i < 50; i++ {
		options = append(options, fmt.Sprintf("Type %02d", i))
	}
	p := newIssuesFilterPanel([]issueFilterSection{{Title: "Types", Options: options, AllowAll: true}})
	for i := 0; i < 36; i++ {
		p.move("down")
	}
	view := p.View(30, 6)
	if !strings.Contains(ansi.Strip(view), "> [ ] Type 35") {
		t.Fatalf("current cursor must stay visible after scrolling: %q", ansi.Strip(view))
	}
	if lipgloss.Height(view) > 6 {
		t.Fatal("long options must remain within the viewport")
	}
	p.move("down")
	for i := 0; i < 37; i++ {
		p.move("up")
	}
	if !strings.Contains(ansi.Strip(p.View(30, 6)), "> [x] All types") {
		t.Fatal("scrolling back must reveal the All types row")
	}
}

func TestIssuesFilterPanelClipsUnicodeAndMultilineOptions(t *testing.T) {
	p := newIssuesFilterPanel([]issueFilterSection{{
		Title: "Types", Options: []string{"開発エピック\nwith a long label"},
	}})
	view := p.View(14, 3)
	if lipgloss.Width(view) > 14 || lipgloss.Height(view) > 3 {
		t.Fatalf("option content escaped the viewport: %q", ansi.Strip(view))
	}
}
