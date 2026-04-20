//go:build darwin

package manager

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"
)

const tunneldHost = "127.0.0.1"
const tunneldPort = 49151

// IsTunneldRunning returns true if pymobiledevice3 tunneld is already listening.
func IsTunneldRunning() bool {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", tunneldHost, tunneldPort), 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// StartTunneldWithAdmin asks for the admin password via macOS's built-in
// AppleScript prompt, then starts `pymobiledevice3 remote tunneld --daemonize`
// as root so it can create the utun interface required for RemoteXPC tunnels.
// Returns nil if tunneld is already running or starts successfully.
func StartTunneldWithAdmin() error {
	if IsTunneldRunning() {
		return nil
	}

	py, err := resolvePipxPython()
	if err != nil {
		return err
	}

	// The pipx-installed pymobiledevice3 CLI shim.
	cli := strings.Replace(py, "/bin/python", "/bin/pymobiledevice3", 1)

	// AppleScript strings use double quotes. Embed the shell command as an
	// AppleScript string literal (escape backslashes + double quotes inside).
	shellCmd := fmt.Sprintf(
		`%s remote tunneld --daemonize --host %s --port %d`,
		cli, tunneldHost, tunneldPort,
	)
	osa := fmt.Sprintf(
		`do shell script %s with administrator privileges with prompt %s`,
		appleScriptString(shellCmd),
		appleScriptString("iNoaload needs admin rights to start the tvOS install tunnel daemon."),
	)

	cmd := exec.Command("osascript", "-e", osa)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("osascript failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	// Wait up to 5s for the port to open.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if IsTunneldRunning() {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("tunneld did not start within 5s")
}

// appleScriptString returns an AppleScript string literal (double-quoted,
// with backslashes and double quotes escaped).
func appleScriptString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
