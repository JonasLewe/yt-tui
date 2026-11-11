package ui

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type editorCaptureModel struct {
	command tea.Cmd
	result  editorFinishedMsg
	done    bool
}

func (m editorCaptureModel) Init() tea.Cmd { return m.command }
func (m editorCaptureModel) View() string  { return "" }
func (m editorCaptureModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if result, ok := msg.(editorFinishedMsg); ok {
		m.result, m.done = result, true
		return m, tea.Quit
	}
	return m, nil
}

func runExternalEditor(t *testing.T, command tea.Cmd) editorFinishedMsg {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	program := tea.NewProgram(editorCaptureModel{command: command},
		tea.WithContext(ctx), tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard),
		tea.WithoutRenderer(), tea.WithoutSignalHandler())
	model, err := program.Run()
	if err != nil {
		t.Fatalf("run editor handoff: %v", err)
	}
	result := model.(editorCaptureModel)
	if !result.done {
		t.Fatal("editor did not deliver its completion message")
	}
	if result.result.tempPath != "" {
		t.Cleanup(func() { _ = os.Remove(result.result.tempPath) })
	}
	return result.result
}

func fakeExternalEditor(t *testing.T, name string) (path, arguments, before string) {
	t.Helper()
	dir := t.TempDir()
	path = filepath.Join(dir, name)
	arguments = filepath.Join(dir, "arguments")
	before = filepath.Join(dir, "before")
	script := `#!/bin/sh
printf '%s\n' "$@" > "$YT_EDITOR_ARGUMENTS"
for editor_arg do editor_file="$editor_arg"; done
cat "$editor_file" > "$YT_EDITOR_BEFORE"
printf '%s' "$YT_EDITOR_CONTENT" > "$editor_file"
exit "${YT_EDITOR_EXIT:-0}"
`
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YT_EDITOR_ARGUMENTS", arguments)
	t.Setenv("YT_EDITOR_BEFORE", before)
	t.Setenv("YT_EDITOR_CONTENT", "Edited **Markdown**\nsecond line\n")
	t.Setenv("YT_EDITOR_EXIT", "0")
	return path, arguments, before
}

func readEditorFixture(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func assertEditorFileRemoved(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("editor temporary file was not removed: %v", err)
	}
}

func TestExternalEditorKeepsIssueFormWorkflow(t *testing.T) {
	path, arguments, before := fakeExternalEditor(t, "editor")
	t.Setenv("EDITOR", path+" --wait --clean")
	m := newFormModel(nil, nil)
	const original = "# Original\n\nDraft content.\n"
	m.descTextArea.SetValue(original)
	result := runExternalEditor(t, m.openEditorCmd())
	if result.err != nil || result.readOnly {
		t.Fatalf("unexpected editor result: %+v", result)
	}
	if got := readEditorFixture(t, before); got != original {
		t.Fatalf("editor received %q, want %q", got, original)
	}
	wantArgs := []string{"--wait", "--clean", result.tempPath}
	if got := strings.Split(strings.TrimSuffix(readEditorFixture(t, arguments), "\n"), "\n"); !reflect.DeepEqual(got, wantArgs) {
		t.Fatalf("editor arguments = %q, want %q", got, wantArgs)
	}
	if !strings.HasPrefix(filepath.Base(result.tempPath), "yt-tui-desc-") || filepath.Ext(result.tempPath) != ".md" {
		t.Fatalf("unexpected issue description temp path: %s", result.tempPath)
	}
	if m.descTextArea.Value() != original {
		t.Fatal("editor modified the form before its completion was handled")
	}
	m, _ = m.Update(result)
	if got := m.descTextArea.Value(); got != os.Getenv("YT_EDITOR_CONTENT") {
		t.Fatalf("form did not receive the edited draft: %q", got)
	}
	assertEditorFileRemoved(t, result.tempPath)
}

func TestExternalEditorDefaultsToVim(t *testing.T) {
	path, _, before := fakeExternalEditor(t, "vim")
	t.Setenv("EDITOR", " \t ")
	t.Setenv("PATH", filepath.Dir(path)+string(os.PathListSeparator)+os.Getenv("PATH"))
	result := runExternalEditor(t, openTextEditorCmd("Article draft", "yt-tui-article-*.md", false))
	if result.err != nil {
		t.Fatal(result.err)
	}
	if got := readEditorFixture(t, before); got != "Article draft" {
		t.Fatalf("fallback editor received %q", got)
	}
	if got := readEditorFixture(t, result.tempPath); got != os.Getenv("YT_EDITOR_CONTENT") {
		t.Fatalf("fallback editor did not update the draft: %q", got)
	}
}

func TestExternalEditorKeepsIssueDetailReadOnlyBehavior(t *testing.T) {
	path, _, _ := fakeExternalEditor(t, "editor")
	t.Setenv("EDITOR", path)
	for _, readOnly := range []bool{false, true} {
		name := "comment"
		if readOnly {
			name = "view"
		}
		t.Run(name, func(t *testing.T) {
			m := newDetailModel(nil, nil)
			m.commentInput.SetValue("Existing comment draft")
			result := runExternalEditor(t, m.openEditorCmd("Article or issue text", readOnly))
			if result.err != nil || result.readOnly != readOnly {
				t.Fatalf("unexpected editor result: %+v", result)
			}
			if !strings.HasPrefix(filepath.Base(result.tempPath), "yt-tui-"+name+"-") {
				t.Fatalf("unexpected temporary file prefix: %s", result.tempPath)
			}
			m, _ = m.Update(result)
			want := os.Getenv("YT_EDITOR_CONTENT")
			if readOnly {
				want = "Existing comment draft"
			}
			if got := m.commentInput.Value(); got != want {
				t.Fatalf("comment draft = %q, want %q", got, want)
			}
			assertEditorFileRemoved(t, result.tempPath)
		})
	}
}

func TestExternalEditorFailurePreservesDraftAndCleansUp(t *testing.T) {
	path, _, _ := fakeExternalEditor(t, "editor")
	t.Setenv("EDITOR", path)
	t.Setenv("YT_EDITOR_EXIT", "3")
	m := newFormModel(nil, nil)
	m.descTextArea.SetValue("Keep this draft")
	result := runExternalEditor(t, m.openEditorCmd())
	if result.err == nil {
		t.Fatal("failed editor did not return its error")
	}
	m, _ = m.Update(result)
	if m.err == nil || m.descTextArea.Value() != "Keep this draft" {
		t.Fatal("failed editor replaced the draft or lost the error")
	}
	assertEditorFileRemoved(t, result.tempPath)
}
