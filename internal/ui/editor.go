package ui

import (
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// openTextEditorCmd uses the same editor handoff for issue and article content.
// The receiving model reads and removes the temporary file after the editor exits.
func openTextEditorCmd(content, prefix string, readOnly bool) tea.Cmd {
	editorParts := strings.Fields(os.Getenv("EDITOR"))
	if len(editorParts) == 0 {
		editorParts = []string{"vim"}
	}

	tempFile, err := os.CreateTemp("", prefix)
	if err != nil {
		return func() tea.Msg {
			return editorFinishedMsg{err: err}
		}
	}

	if _, err := tempFile.WriteString(content); err != nil {
		tempFile.Close()
		os.Remove(tempFile.Name())
		return func() tea.Msg {
			return editorFinishedMsg{err: err}
		}
	}
	tempFile.Close()

	bin := editorParts[0]
	args := append(editorParts[1:], tempFile.Name())
	c := exec.Command(bin, args...)
	return execEditorProcess(c, func(err error) tea.Msg {
		return editorFinishedMsg{tempPath: tempFile.Name(), err: err, readOnly: readOnly}
	})
}
