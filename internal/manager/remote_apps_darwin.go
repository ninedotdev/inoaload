//go:build darwin

package manager

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// UninstallRemoteApp removes a bundle from a tvOS 17+ / iOS 17+ device via
// pymobiledevice3 apps uninstall. Best-effort: returns an error if the call
// fails, but callers usually ignore it (e.g., before a reinstall).
func UninstallRemoteApp(ctx context.Context, mdnsID, deviceName, bundleID string) error {
	if bundleID == "" {
		return fmt.Errorf("bundle_identifier is empty")
	}

	resolveCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resolution, err := ResolveTunnel(resolveCtx, mdnsID, deviceName)
	if err != nil {
		return err
	}

	py, err := resolvePipxPython()
	if err != nil {
		return err
	}
	cli := strings.Replace(py, "/bin/python", "/bin/pymobiledevice3", 1)

	runCtx, runCancel := context.WithTimeout(ctx, 30*time.Second)
	defer runCancel()
	cmd := exec.CommandContext(runCtx, cli,
		"apps", "uninstall", "--tunnel", resolution.HardwareUDID, bundleID)
	cmd.Env = augmentedPath(os.Environ())
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("apps uninstall failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}
