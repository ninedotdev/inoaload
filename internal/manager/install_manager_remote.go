//go:build darwin

package manager

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bitxeno/atvloadly/internal/log"
)

var remoteUDIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// RemoteInstallManager handles installation to tvOS 17+ / iOS 17+ devices
// that speak the new RemoteXPC protocol. The flow is:
//  1. plumesign sign --apple-id … -o signed.ipa   (legacy signing, still works)
//  2. pymobiledevice3 apps install --tunnel <udid> signed.ipa
//
// Step 2 needs a running tunneld daemon, which must be started with admin
// privileges via StartTunneldWithAdmin beforehand.
type RemoteInstallManager struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	onOutput func(string)
	lastErr  string
	teamID   string
}

func NewRemoteInstallManager() *RemoteInstallManager {
	return &RemoteInstallManager{}
}

func (m *RemoteInstallManager) OnOutput(fn func(string)) { m.onOutput = fn }

func (m *RemoteInstallManager) Write(p []byte) {
	if m.stdin != nil {
		_, _ = m.stdin.Write(p)
	}
}

func (m *RemoteInstallManager) Close() {
	if m.stdin != nil {
		_ = m.stdin.Close()
	}
	if m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Process.Kill()
	}
}

// IsRemoteUDID returns true when the UDID has the shape of a RemoteXPC
// identifier (standard 8-4-4-4-12 UUID). Legacy usbmuxd UDIDs do not match.
func IsRemoteUDID(udid string) bool {
	return remoteUDIDPattern.MatchString(udid)
}

func (m *RemoteInstallManager) Start(ctx context.Context, opts InstallOptions) error {
	m.teamID = opts.TeamID
	if !IsTunneldRunning() {
		m.emit("ERROR: tunneld daemon is not running. Start it from the Settings tab.")
		return fmt.Errorf("tunneld not running")
	}
	if opts.TeamID != "" {
		m.emit("→ Using Apple Developer team: " + opts.TeamID)
	}

	// Resolve the real Apple hardware UDID (Apple's developer API won't accept
	// the mDNS identifier). Keep the mDNS id to reference the tunnel in
	// pymobiledevice3 apps install.
	m.emit("→ Waiting for tunneld to reach the TV…")
	resolution, err := ResolveTunnel(ctx, opts.UDID, opts.DeviceName)
	if err != nil {
		m.emit("ERROR: could not resolve hardware UDID: " + err.Error())
		return err
	}
	opts.UDID = resolution.HardwareUDID
	m.emit(fmt.Sprintf("→ Resolved: hw=%s tunnel=%s", resolution.HardwareUDID, resolution.TunnelKey))

	plumesignPath, err := ResolvePlumesign()
	if err != nil {
		m.emit("ERROR: " + err.Error())
		return err
	}

	// Phase 0: ensure the Apple ID is logged in. Plumesign stores the developer
	// session locally; `sign --apple-id` needs a prior account login.
	if !m.accountRegistered(ctx, plumesignPath, opts.Account) {
		m.emit("→ Phase 0/2: logging in to Apple ID (2FA may be requested)")
		loginArgs := []string{"account", "login", "-u", opts.Account, "-p", opts.Password}
		if err := m.runStreaming(ctx, plumesignPath, loginArgs); err != nil {
			return err
		}
	}

	signedPath := filepath.Join(os.TempDir(), "iNoaload", "signed.ipa")
	_ = os.MkdirAll(filepath.Dir(signedPath), 0o755)
	_ = os.Remove(signedPath)

	// Phase 0.5: register the device with the developer team. Apple requires at
	// least one device on the team before `downloadTeamProvisioningProfile`.
	// plumesign's auto-register path only works via the legacy AFC flow, so we
	// invoke register-device explicitly here (it's idempotent on the server).
	deviceName := "AppleTV"
	if opts.UDID != "" {
		regArgs := []string{
			"account", "register-device",
			"-u", opts.Account,
			"--udid", opts.UDID,
			"--name", deviceName,
			"--platform", "tvos",
		}
		if opts.TeamID != "" {
			regArgs = append(regArgs, "-t", opts.TeamID)
		}
		m.emit("→ Registering device with developer team")
		_ = m.runStreaming(ctx, plumesignPath, regArgs) // ignore error — device may already be registered
	}

	// Phase 1: sign the IPA via plumesign
	signArgs := []string{
		"sign", "--apple-id",
		"-u", opts.Account,
		"-p", opts.IpaPath,
		"-o", signedPath,
	}
	if opts.RemoveExtensions {
		signArgs = append(signArgs, "--remove-extensions")
	}
	m.emit("→ Phase 1/2: signing IPA")
	if err := m.runStreaming(ctx, plumesignPath, signArgs); err != nil {
		return err
	}
	if _, err := os.Stat(signedPath); err != nil {
		m.emit(fmt.Sprintf("ERROR: signed IPA not produced at %s", signedPath))
		return err
	}

	// Phase 2: push to device via pymobiledevice3 apps install. Re-resolve the
	// tunnel key first because the tvOS RemoteXPC identifier can rotate during
	// the long signing phase, invalidating the one we captured earlier.
	py, err := resolvePipxPython()
	if err != nil {
		m.emit("ERROR: " + err.Error())
		return err
	}
	cli := strings.Replace(py, "/bin/python", "/bin/pymobiledevice3", 1)

	// pymobiledevice3 `--tunnel` expects the device's hardware UDID (it looks
	// up the RSD by that), not the mDNS tunneld key. The hardware UDID does
	// NOT rotate with tvOS pair mode transitions, so no refresh needed.
	pushArgs := []string{"apps", "install", "--tunnel", resolution.HardwareUDID, signedPath}
	m.emit("→ Phase 2/2: pushing signed IPA via RemoteXPC tunnel")
	if err := m.runStreaming(ctx, cli, pushArgs); err != nil {
		return err
	}
	// pymobiledevice3 exits 0 even when it logs "Device not found" to stderr —
	// treat that string as a hard failure.
	if strings.Contains(strings.ToLower(m.lastErr), "device not found") || strings.Contains(strings.ToLower(m.lastErr), "error ") {
		m.emit("Installation Failed!")
		return fmt.Errorf("apps install reported: %s", m.lastErr)
	}
	m.emit("Installation Succeeded!")
	return nil
}

func (m *RemoteInstallManager) accountRegistered(ctx context.Context, plumesignPath, email string) bool {
	cmd := exec.CommandContext(ctx, plumesignPath, "account", "list")
	cmd.Env = augmentedPath(os.Environ())
	out, _ := cmd.CombinedOutput()
	return strings.Contains(strings.ToLower(string(out)), strings.ToLower(email))
}

func (m *RemoteInstallManager) runStreaming(ctx context.Context, bin string, args []string) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	env := augmentedPath(os.Environ())
	if m.teamID != "" {
		env = append(env, "PLUMESIGN_TEAM_ID="+m.teamID)
	}
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	m.cmd = cmd
	m.stdin = stdin

	go m.pipe(stdout, false)
	go m.pipe(stderr, true)

	if err := cmd.Start(); err != nil {
		log.Err(err).Msg("subprocess start failed")
		return err
	}
	if err := cmd.Wait(); err != nil {
		m.emit(fmt.Sprintf("subprocess exited: %s", err.Error()))
		return err
	}
	return nil
}

func (m *RemoteInstallManager) pipe(r io.Reader, isErr bool) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if isErr {
			m.lastErr = line
		}
		m.emit(line)
	}
}

func (m *RemoteInstallManager) emit(line string) {
	if m.onOutput != nil {
		m.onOutput(line)
	}
}
