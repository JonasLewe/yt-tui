package ui

import (
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// The Neovim launcher opts in for its terminal job only. Standalone terminals
// retain the usual q-to-quit behavior. No input keys are intercepted by the host.
const neovimHideEnvironment = "YT_TUI_NVIM_HIDE"
const neovimHideSequence = "\x1b]777;yt-tui;hide\x07"

type terminalHideRequestMsg struct{}
type terminalHideErrorMsg struct{ err error }

func neovimHostEnabled() bool {
	return os.Getenv(neovimHideEnvironment) == "1"
}

func quitOrHide() tea.Cmd {
	if neovimHostEnabled() {
		return func() tea.Msg { return terminalHideRequestMsg{} }
	}
	return tea.Quit
}

func sendTerminalHide(w io.Writer) tea.Cmd {
	return func() tea.Msg {
		n, err := io.WriteString(w, neovimHideSequence)
		if err == nil && n != len(neovimHideSequence) {
			err = io.ErrShortWrite
		}
		if err != nil {
			return terminalHideErrorMsg{err: err}
		}
		return nil
	}
}

func quitHelp() string {
	if neovimHostEnabled() {
		return "[q] Hide  [Q] Quit"
	}
	return "[q/Q] Quit"
}

func quitKeyDescription() string {
	if neovimHostEnabled() {
		return "Hide float (browsing)"
	}
	return "Quit (browsing)"
}

// Quit keys belong to browsing screens. Text inputs, action shortcuts and
// nested overlays retain their own q/Q behavior instead of hiding the host.
func (m AppModel) canUseQuitKeys() bool {
	switch m.state {
	case stateWelcome:
		return m.welcome.selectingServer && !m.welcome.checking &&
			!m.welcome.loggedIn && m.welcome.err == nil
	case stateDashboard:
		return !m.dashboard.actionMode
	case stateProjects, stateBoards:
		return true
	case stateIssues:
		return !m.issues.searchMode && !m.issues.actionMode
	case stateDetail:
		return m.detail.mode == modeNormal && !m.detail.filepickerActive && !m.detail.errPopupShow
	default:
		return false
	}
}
