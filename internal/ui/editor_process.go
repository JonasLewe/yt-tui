package ui

import (
	"io"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

// Bubble Tea v1 expects legacy key encoding. External editors can leave Kitty
// or modifyOtherKeys enabled on the alternate screen, even after a clean exit.
const legacyEditorKeyboard = "\x1b[=0u\x1b[>4;0m"

func execEditorProcess(command *exec.Cmd, finished tea.ExecCallback) tea.Cmd {
	return tea.ExecProcess(command, func(err error) tea.Msg {
		// ExecProcess invokes this callback after RestoreTerminal has reentered
		// the alternate screen. Resetting before that would affect the wrong
		// screen. Use Bubble Tea's output, which it assigns to the command.
		if command.Stdout != nil {
			_, _ = io.WriteString(command.Stdout, legacyEditorKeyboard)
		}
		if finished == nil {
			return nil
		}
		return finished(err)
	})
}
