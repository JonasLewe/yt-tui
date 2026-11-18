package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"yt-tui/internal/config"
	"yt-tui/internal/ytcli"
)

func TestCreateLinkedIssueFromDetail(t *testing.T) {
	detail := newDetailModel(nil, &config.Config{})
	detail.width, detail.height = 100, 40
	detail.loading = false
	detail.issue = &ytcli.Issue{
		ID: "2-41", IDReadable: "DEV-42", Summary: "Parent epic",
		Project: &ytcli.Project{ShortName: "DEV"},
	}
	_, cmd := detail.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if cmd == nil {
		t.Fatal("expected n to navigate to a linked issue form")
	}
	push, ok := cmd().(pushStateMsg)
	if !ok || push.state != stateForm {
		t.Fatalf("unexpected navigation: %#v", push)
	}
	form := newFormModel(nil, &config.Config{})
	form.width, form.height = 100, 40
	form.setupForm(push.data)
	form, _ = form.Update(formLinkTypesMsg{request: form.links.request, types: []ytcli.IssueLinkType{{
		ID: "80-3", Name: "Subtask", Directed: true,
		SourceToTarget: "parent for", TargetToSource: "subtask of", LocalizedTargetToSource: "gehört zu",
	}}})
	if !strings.Contains(form.View(), "DEV-42") || !strings.Contains(form.View(), "gehört zu") {
		t.Fatalf("picker lacks relation or target: %s", form.View())
	}
	form, _ = form.Update(tea.KeyMsg{Type: tea.KeyDown})
	form, _ = form.Update(tea.KeyMsg{Type: tea.KeyEnter})
	want := formIssueLink{RelationID: "80-3t", Label: "gehört zu", TargetID: "2-41", TargetReadable: "DEV-42"}
	if len(form.links.selected) != 1 || form.links.selected[0] != want || form.links.open() {
		t.Fatalf("selected link: %+v", form.links)
	}
	if form.initialProjectCode != "DEV" || form.summaryInput.Value() != "" {
		t.Fatalf("linked form did not initialize as a new issue in the original project")
	}
}

func TestCreateLinkSearchAndRemoval(t *testing.T) {
	form := newFormModel(nil, &config.Config{})
	form.setupForm("DEV")
	form.blurCurrent()
	form.focusIndex = fieldLinks
	press := func(key tea.KeyMsg) {
		form, _ = form.Update(key)
	}
	enter := tea.KeyMsg{Type: tea.KeyEnter}
	press(enter)
	form, _ = form.Update(formLinkTypesMsg{request: form.links.request, types: []ytcli.IssueLinkType{{
		ID: "80-0", Name: "Related", SourceToTarget: "relates to",
	}}})
	press(enter)
	press(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("MY-PROJ-14")})
	press(enter)
	request := form.links.request
	press(tea.KeyMsg{Type: tea.KeyBackspace})
	form, _ = form.Update(formLinkTargetsMsg{request: request, issues: []ytcli.Issue{{ID: "old"}}})
	if len(form.links.results) != 0 {
		t.Fatal("results for an outdated query were accepted")
	}
	press(enter)
	form, _ = form.Update(formLinkTargetsMsg{
		request: form.links.request,
		issues:  []ytcli.Issue{{ID: "2-1", IDReadable: "MY-PROJ-1", Summary: "Target"}},
	})
	press(enter)
	want := formIssueLink{RelationID: "80-0", Label: "relates to", TargetID: "2-1", TargetReadable: "MY-PROJ-1"}
	if len(form.links.selected) != 1 || form.links.selected[0] != want {
		t.Fatalf("selected links: %+v", form.links.selected)
	}
	press(enter)
	press(tea.KeyMsg{Type: tea.KeyDown})
	press(tea.KeyMsg{Type: tea.KeyDelete})
	if len(form.links.selected) != 0 {
		t.Fatal("delete did not remove the selected link")
	}
}

func TestCancelLinkedCreateAndIgnorePreviousRequest(t *testing.T) {
	form := newFormModel(nil, &config.Config{})
	form.setupForm("linked:DEV:2-5:DEV-6")
	request := form.links.request
	form, cmd := form.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected cancel to navigate back to source issue")
	}
	if _, ok := cmd().(popStateMsg); !ok {
		t.Fatal("expected popStateMsg on cancellation")
	}
	form.setupForm("clone:DEV-7")
	form, _ = form.Update(formLinkTypesMsg{request: request, types: []ytcli.IssueLinkType{{ID: "old"}}})
	if form.links.open() || len(form.links.relations) != 0 || len(form.links.selected) != 0 {
		t.Fatal("previous linked creation leaked into another form")
	}
}

func TestLinkFieldIsReachableOnCompactNewIssueForm(t *testing.T) {
	m := newFormModel(nil, nil)
	m.width, m.height = 56, 19
	m.applyInputSizes()
	for range numFields {
		if m.focusIndex == fieldLinks {
			break
		}
		m.nextField()
	}
	if m.focusIndex != fieldLinks || !strings.Contains(m.View(), "Links:") || lipgloss.Height(m.View()) > m.height {
		t.Fatalf("link field is not visible at keyboard focus: %s", m.View())
	}
	m.isEdit = true
	if m.canFocusField(fieldLinks) {
		t.Fatal("link field is available during edit")
	}
	m.isEdit, m.isClone = false, true
	if m.canFocusField(fieldLinks) {
		t.Fatal("link field is available during clone")
	}
}
