package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestFormScrollsWithKeyboardFocus(t *testing.T) {
	m := newFormModel(nil, nil)
	m.width, m.height = 56, 19
	m.initialProjectCode = "APP"
	m.applyInputSizes()
	expected := map[formField]string{
		fieldProject: "Project:", fieldSummary: "Summary:", fieldDescription: "Description:",
		fieldType: "Type:", fieldPriority: "Priority:", fieldAssignee: "Assignee:",
	}
	for range numFields {
		if label, ok := expected[m.focusIndex]; ok {
			view := m.View()
			if !strings.Contains(view, label) || lipgloss.Height(view) > m.height {
				t.Fatalf("focused %s at %d x %d:\n%s", label, m.width, m.height, view)
			}
			delete(expected, m.focusIndex)
		}
		m.nextField()
	}
	if len(expected) != 0 {
		t.Fatalf("some standard fields were unreachable: %v", expected)
	}
}

func TestEditNavigationKeepsProjectReadOnly(t *testing.T) {
	m := newFormModel(nil, nil)
	m.isEdit = true
	m.focusIndex = fieldSummary
	m.prevField()
	if m.focusIndex != fieldAssignee {
		t.Fatalf("shift-tab from Summary focused %d", m.focusIndex)
	}
	m.nextField()
	if m.focusIndex != fieldSummary {
		t.Fatalf("tab from Assignee focused %d", m.focusIndex)
	}
}
