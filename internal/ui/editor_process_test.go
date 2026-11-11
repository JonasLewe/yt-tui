package ui

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const editorProcessExpectedKeyboardReset = "\x1b[=0u\x1b[>4;0m"

// The real Bubble Tea renderer writes from another goroutine after restoring
// the terminal, so both the transcript and callback snapshot need a lock.
type editorProcessOutput struct {
	mu            sync.Mutex
	transcript    strings.Builder
	failReset     bool
	resetAttempts int
	resetOffset   int
}

func (w *editorProcessOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if string(p) == editorProcessExpectedKeyboardReset {
		w.resetAttempts++
		w.resetOffset = w.transcript.Len()
		if w.failReset {
			return 0, io.ErrClosedPipe
		}
	}
	return w.transcript.Write(p)
}

func (w *editorProcessOutput) snapshot() editorProcessResult {
	w.mu.Lock()
	defer w.mu.Unlock()
	return editorProcessResult{
		transcript: w.transcript.String(), resetAttempts: w.resetAttempts, resetOffset: w.resetOffset,
	}
}

type editorProcessResult struct {
	err           error
	transcript    string
	resetAttempts int
	resetOffset   int
}

type editorProcessTestModel struct {
	command  tea.Cmd
	finished bool
	result   editorProcessResult
}

func (m editorProcessTestModel) Init() tea.Cmd { return m.command }
func (m editorProcessTestModel) View() string  { return "Editor handoff regression" }
func (m editorProcessTestModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if result, ok := msg.(editorProcessResult); ok {
		m.result, m.finished = result, true
		return m, tea.Quit
	}
	return m, nil
}

func TestEditorProcessRestoresKeyboardAfterAlternateScreen(t *testing.T) {
	cases := []struct {
		name    string
		command func(*testing.T) *exec.Cmd
		check   func(*testing.T, error)
	}{
		{
			name: "success",
			command: func(*testing.T) *exec.Cmd {
				return exec.Command("sh", "-c", "printf '\033[>1u\033[>4;2mEDITOR_RAN'")
			},
			check: func(t *testing.T, err error) {
				if err != nil {
					t.Fatalf("successful editor acquired an error: %v", err)
				}
			},
		},
		{
			name: "editor exits with error",
			command: func(*testing.T) *exec.Cmd {
				return exec.Command("sh", "-c", "printf '\033[>1u\033[>4;2mEDITOR_RAN'; exit 7")
			},
			check: func(t *testing.T, err error) {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
					t.Fatalf("editor exit error was lost: %v", err)
				}
			},
		},
		{
			name: "editor executable missing",
			command: func(t *testing.T) *exec.Cmd {
				return exec.Command(filepath.Join(t.TempDir(), "missing-editor"))
			},
			check: func(t *testing.T, err error) {
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("editor start error was lost: %v", err)
				}
			},
		},
	}

	for _, tc := range cases {
		for _, failReset := range []bool{false, true} {
			name := tc.name
			if failReset {
				name += "/reset write fails"
			}
			t.Run(name, func(t *testing.T) {
				output := &editorProcessOutput{failReset: failReset}
				command := tc.command(t)
				wrapped := execEditorProcess(command, func(err error) tea.Msg {
					// Capture at callback time, before the model consumes the
					// completion and before Program shutdown exits alt screen.
					result := output.snapshot()
					result.err = err
					return result
				})
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				program := tea.NewProgram(editorProcessTestModel{command: wrapped},
					tea.WithContext(ctx), tea.WithInput(strings.NewReader("")), tea.WithOutput(output),
					tea.WithAltScreen(), tea.WithoutSignalHandler())
				model, err := program.Run()
				if err != nil {
					t.Fatalf("editor handoff program failed: %v", err)
				}
				finished := model.(editorProcessTestModel)
				if !finished.finished {
					t.Fatal("editor completion callback did not reach the model")
				}
				result := finished.result
				tc.check(t, result.err)
				if command.Stdout != output {
					t.Fatal("editor reset did not use Bubble Tea's output writer")
				}
				if result.resetAttempts != 1 {
					t.Fatalf("reset attempts before callback = %d, want 1", result.resetAttempts)
				}

				const enterAltScreen = "\x1b[?1049h"
				const leaveAltScreen = "\x1b[?1049l"
				if strings.Count(result.transcript, enterAltScreen) != 2 {
					t.Fatalf("expected initial alternate screen and restored alternate screen before callback: %q", result.transcript)
				}
				firstEnter := strings.Index(result.transcript, enterAltScreen)
				leave := strings.Index(result.transcript, leaveAltScreen)
				reenter := strings.LastIndex(result.transcript, enterAltScreen)
				if firstEnter >= leave || leave >= reenter || reenter >= result.resetOffset {
					t.Fatalf("keyboard reset happened before the TUI's alternate screen was restored: %q", result.transcript)
				}
				if !failReset {
					if strings.Count(result.transcript, editorProcessExpectedKeyboardReset) != 1 ||
						strings.Index(result.transcript, editorProcessExpectedKeyboardReset) != result.resetOffset {
						t.Fatalf("both legacy keyboard resets were not emitted together before callback: %q", result.transcript)
					}
				}
			})
		}
	}
}
