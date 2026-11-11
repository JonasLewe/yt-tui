package ui

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"yt-tui/internal/config"
	"yt-tui/internal/ytcli"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func quitKey(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

func TestBrowsingQuitKeysRespectTerminalHost(t *testing.T) {
	for _, host := range []string{"", "0", "1"} {
		t.Run("host="+host, func(t *testing.T) {
			t.Setenv(neovimHideEnvironment, host)
			for _, state := range []State{stateDashboard, stateProjects, stateBoards, stateIssues, stateDetail, stateWelcome, stateArticles, stateArticleDetail} {
				for _, key := range []string{"q", "Q"} {
					t.Run(string(state)+"/"+key, func(t *testing.T) {
						app := AppModel{state: state, stateData: "TEST-1", history: []navEntry{{state: stateDashboard}}}
						app.welcome.selectingServer = true
						app.detail.activeViewport = 2
						app.detail.linksCursor = 3
						updated, cmd := app.Update(quitKey(key))
						if !reflect.DeepEqual(updated, app) {
							t.Fatal("quit key changed browsing/navigation state")
						}
						if cmd == nil {
							t.Fatal("quit key returned no command")
						}
						msg := cmd()
						if host == "1" && key == "q" {
							if _, ok := msg.(terminalHideRequestMsg); !ok {
								t.Fatalf("got %T, want host hide request", msg)
							}
						} else if _, ok := msg.(tea.QuitMsg); !ok {
							t.Fatalf("got %T, want quit", msg)
						}
					})
				}
			}
		})
	}
}

func TestQuitLettersRemainTextInInputs(t *testing.T) {
	t.Setenv(neovimHideEnvironment, "1")
	tests := []struct {
		name  string
		setup func() AppModel
		value func(AppModel) string
	}{
		{"issue filter", func() AppModel {
			m := AppModel{state: stateIssues, issues: newIssuesModel(nil, nil)}
			m.issues.searchMode = true
			m.issues.loading = false
			return m
		}, func(m AppModel) string { return m.issues.searchInput.Value() }},
		{"article filter", func() AppModel {
			m := AppModel{state: stateArticles, articles: newArticlesModel(nil, nil)}
			m.articles.searchMode = true
			m.articles.searchInput.Focus()
			return m
		}, func(m AppModel) string { return m.articles.searchInput.Value() }},
		{"article summary", func() AppModel {
			m := AppModel{state: stateArticleForm, articleForm: newArticleFormModel(nil, nil)}
			m.articleForm.setupForm("new:DEMO")
			return m
		}, func(m AppModel) string { return m.articleForm.summaryInput.Value() }},
		{"article content", func() AppModel {
			m := AppModel{state: stateArticleForm, articleForm: newArticleFormModel(nil, nil)}
			m.articleForm.setupForm("new:DEMO")
			m.articleForm.focusIndex = articleFieldContent
			m.articleForm.focusCurrent()
			return m
		}, func(m AppModel) string { return m.articleForm.contentInput.Value() }},
		{"global search", func() AppModel {
			input := textinput.New()
			input.Focus()
			return AppModel{state: stateDashboard, searchShow: true, searchInInputMode: true, searchInput: input}
		}, func(m AppModel) string { return m.searchInput.Value() }},
		{"issue description", func() AppModel {
			m := AppModel{state: stateForm, form: newFormModel(nil, nil)}
			m.form.focusIndex = fieldDescription
			m.form.descTextArea.Focus()
			return m
		}, func(m AppModel) string { return m.form.descTextArea.Value() }},
		{"comment", func() AppModel {
			m := AppModel{state: stateDetail, detail: newDetailModel(nil, nil)}
			m.detail.loading = false
			m.detail.mode = modeCommentInput
			m.detail.commentInput.Focus()
			return m
		}, func(m AppModel) string { return m.detail.commentInput.Value() }},
		{"assignee", func() AppModel {
			m := AppModel{state: stateDetail, detail: newDetailModel(nil, nil)}
			m.detail.loading = false
			m.detail.mode = modeAssignInput
			m.detail.textInput.Focus()
			return m
		}, func(m AppModel) string { return m.detail.textInput.Value() }},
		{"credentials", func() AppModel {
			m := AppModel{state: stateWelcome, welcome: newWelcomeModel(&ytcli.Client{}, nil, nil, "")}
			m.welcome.checking = false
			return m
		}, func(m AppModel) string { return m.welcome.urlInput.Value() }},
		{"zoxide query", func() AppModel {
			m := AppModel{state: stateDetail, detail: newDetailModel(nil, nil)}
			m.detail.filepickerActive = true
			m.detail.zoxide.active = true
			m.detail.zoxide.input.Focus()
			return m
		}, func(m AppModel) string { return m.detail.zoxide.input.Value() }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := tc.setup()
			for _, key := range []string{"q", "Q"} {
				updated, _ := app.Update(quitKey(key))
				app = updated.(AppModel)
			}
			if got := tc.value(app); got != "qQ" {
				t.Fatalf("typed letters = %q, want qQ", got)
			}
		})
	}
}

func TestQuitLettersKeepCustomActionShortcuts(t *testing.T) {
	t.Setenv(neovimHideEnvironment, "1")
	for _, key := range []string{"q", "Q"} {
		cfg := &config.Config{Actions: config.ActionsMap{"default": {{Name: "Test", Shortcut: key}}}}
		for _, state := range []State{stateDashboard, stateIssues, stateDetail} {
			t.Run(string(state)+"/"+key, func(t *testing.T) {
				issue := ytcli.Issue{IDReadable: "TEST-1"}
				app := AppModel{state: state}
				app.dashboard = newDashboardModel(nil, cfg)
				app.dashboard.actionMode = true
				app.dashboard.issues = []ytcli.Issue{issue}
				app.issues = newIssuesModel(nil, cfg)
				app.issues.actionMode = true
				app.issues.visibleIssueIDs = []string{issue.IDReadable}
				app.issues.table.SetRows([]table.Row{{issue.IDReadable}})
				app.detail = newDetailModel(nil, cfg)
				app.detail.loading = false
				app.detail.mode = modeActionSelect
				app.detail.issue = &issue
				_, cmd := app.Update(quitKey(key))
				if cmd == nil {
					t.Fatal("action shortcut was not dispatched")
				}
				// An action with no commands performs no HTTP calls, while its result
				// proves the action handler received the key instead of the host.
				switch msg := cmd().(type) {
				case dashboardActionFinishedMsg, actionFinishedMsg, detailActionFinishedMsg:
				default:
					t.Fatalf("action shortcut produced %T", msg)
				}
			})
		}
	}
}

func TestQuitLettersRespectOverlaysAndWelcomeState(t *testing.T) {
	t.Setenv(neovimHideEnvironment, "1")
	for _, key := range []string{"q", "Q"} {
		for _, app := range []AppModel{
			{state: stateDashboard, helpShow: true},
			{state: stateDashboard, searchShow: true},
			{state: stateWelcome, welcome: welcomeModel{selectingServer: true, checking: true}},
			{state: stateWelcome, welcome: welcomeModel{selectingServer: true, loggedIn: true}},
			{state: stateWelcome, welcome: welcomeModel{selectingServer: true, err: errors.New("connect failed")}},
		} {
			_, cmd := app.Update(quitKey(key))
			if cmd != nil {
				t.Fatalf("overlay/welcome state intercepted %s", key)
			}
		}
		app := AppModel{state: stateDetail, detail: newDetailModel(nil, nil)}
		app.detail.errPopupShow = true
		app.detail.err = errors.New("save failed")
		updated, cmd := app.Update(quitKey(key))
		if cmd != nil || updated.(AppModel).detail.errPopupShow {
			t.Fatal("error popup did not retain its dismiss behavior")
		}
	}
	app := AppModel{state: stateForm}
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("Ctrl+C no longer quits from an input screen")
	}
}

type failingHideWriter struct{ short bool }

func (w failingHideWriter) Write(p []byte) (int, error) {
	if w.short {
		return len(p) - 1, nil
	}
	return 0, errors.New("closed output")
}

func TestTerminalHideProtocolAndWriteFailures(t *testing.T) {
	var output bytes.Buffer
	if msg := sendTerminalHide(&output)(); msg != nil {
		t.Fatalf("successful hide write returned %T", msg)
	}
	if got := output.String(); got != "\x1b]777;yt-tui;hide\x07" {
		t.Fatalf("unexpected terminal protocol bytes: %q", got)
	}
	for _, short := range []bool{false, true} {
		msg, ok := sendTerminalHide(failingHideWriter{short: short})().(terminalHideErrorMsg)
		if !ok || msg.err == nil {
			t.Fatal("write failure did not report an error")
		}
		if short && !errors.Is(msg.err, io.ErrShortWrite) {
			t.Fatalf("short write error = %v", msg.err)
		}
		app := AppModel{state: stateDetail, stateData: "TEST-1"}
		updated, cmd := app.Update(msg)
		got := updated.(AppModel)
		if cmd != nil || got.state != app.state || got.stateData != app.stateData || !got.isStatusErr || !strings.Contains(got.status, msg.err.Error()) {
			t.Fatal("write failure discarded state or did not display the error")
		}
	}
}

func TestTerminalHostQuitHelp(t *testing.T) {
	for _, host := range []string{"", "1"} {
		t.Setenv(neovimHideEnvironment, host)
		if got := quitHelp(); (host == "1") != strings.Contains(got, "[q] Hide") || !strings.Contains(got, "Quit") {
			t.Fatalf("host %q quit help = %q", host, got)
		}
		app := AppModel{}
		if help := app.renderHelpPopup(); !strings.Contains(help, quitKeyDescription()) {
			t.Fatal("global help omitted active quit behavior")
		}
	}
}
