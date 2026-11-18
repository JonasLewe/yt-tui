package ui

import (
	"fmt"
	"os"
	"testing"
)

// UI tests exercise configuration saves and migrations. Never run them against
// the developer's actual settings, even when a fixture loads config indirectly.
func TestMain(m *testing.M) {
	testHome, err := os.MkdirTemp("", "yt-tui-ui-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create isolated test home:", err)
		os.Exit(1)
	}
	for _, name := range []string{"HOME", "USERPROFILE"} {
		if err := os.Setenv(name, testHome); err != nil {
			_ = os.RemoveAll(testHome)
			fmt.Fprintln(os.Stderr, "configure isolated test home:", err)
			os.Exit(1)
		}
	}
	_ = os.Unsetenv("YOUTRACK_BASE_URL")
	_ = os.Unsetenv("YOUTRACK_TOKEN")
	result := m.Run()
	if err := os.RemoveAll(testHome); err != nil {
		fmt.Fprintln(os.Stderr, "remove isolated test home:", err)
		result = 1
	}
	os.Exit(result)
}
