package ytcli

import (
	"fmt"
	"os"
	"testing"
)

// Client methods may load configuration while resolving credentials or user
// settings. Tests must never read or migrate the developer's real account data.
func TestMain(m *testing.M) {
	testHome, err := os.MkdirTemp("", "yt-tui-client-tests-")
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
