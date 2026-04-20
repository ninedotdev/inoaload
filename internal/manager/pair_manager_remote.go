//go:build darwin

package manager

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bitxeno/atvloadly/internal/log"
)

//go:embed pair_helper.py
var pairHelperSrc []byte

// RemotePairManager wraps pymobiledevice3 to pair tvOS 17+/iOS 17+ devices
// that only speak the new RemoteXPC pairing protocol (_remotepairing._tcp).
// We use an embedded Python helper instead of the bundled CLI because the CLI
// opens an interactive menu to pick an IP, which breaks when stdout is piped.
type RemotePairManager struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	onOutput  func(string)
	lastError string
}

func NewRemotePairManager() *RemotePairManager {
	return &RemotePairManager{}
}

func (m *RemotePairManager) OnOutput(fn func(string)) { m.onOutput = fn }

func (m *RemotePairManager) Start(ctx context.Context, name string) error {
	py, err := resolvePipxPython()
	if err != nil {
		m.emit(fmt.Sprintf("ERROR: %s", err.Error()))
		return err
	}
	m.emit(fmt.Sprintf("using interpreter: %s", py))

	script, err := writeHelperToTemp()
	if err != nil {
		m.emit(fmt.Sprintf("ERROR: write helper: %s", err.Error()))
		return err
	}
	m.emit(fmt.Sprintf("helper at: %s", script))

	cmd := exec.CommandContext(ctx, py, "-u", script, name)
	cmd.Env = augmentedPath(os.Environ())

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
		m.emit(fmt.Sprintf("ERROR: cmd.Start: %s", err.Error()))
		log.Err(err).Msg("failed to start pair_helper")
		return err
	}
	m.emit(fmt.Sprintf("pair_helper started (pid=%d)", cmd.Process.Pid))

	go func() {
		if err := cmd.Wait(); err != nil {
			m.emit(fmt.Sprintf("subprocess exited: %s", err.Error()))
		} else {
			m.emit("subprocess exited cleanly")
		}
	}()

	return nil
}

func (m *RemotePairManager) Write(p []byte) {
	if m.stdin == nil {
		return
	}
	_, _ = m.stdin.Write(p)
}

func (m *RemotePairManager) Close() {
	if m.stdin != nil {
		_ = m.stdin.Close()
	}
	if m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Process.Kill()
	}
}

func (m *RemotePairManager) pipe(r io.Reader, isErr bool) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if isErr {
			m.lastError = line
		}
		m.emit(line)
	}
}

func (m *RemotePairManager) emit(line string) {
	if m.onOutput != nil {
		m.onOutput(line)
	}
}

func writeHelperToTemp() (string, error) {
	dir := filepath.Join(os.TempDir(), "iNoaload")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "pair_helper.py")
	if err := os.WriteFile(path, pairHelperSrc, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// resolvePipxPython returns the interpreter inside the pipx-managed venv of
// pymobiledevice3. That guarantees the library is importable regardless of
// the system Python.
func resolvePipxPython() (string, error) {
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".local", "pipx", "venvs", "pymobiledevice3", "bin", "python"),
		filepath.Join(home, ".local", "share", "pipx", "venvs", "pymobiledevice3", "bin", "python"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	// Fallback: use whichever pymobiledevice3 CLI we can find, resolve its interpreter.
	if bin, err := exec.LookPath("pymobiledevice3"); err == nil {
		// `pipx install` creates a shim; the sibling `python` in its bin dir is the venv python.
		py := filepath.Join(filepath.Dir(bin), "python")
		if _, err := os.Stat(py); err == nil {
			return py, nil
		}
	}
	return "", fmt.Errorf("pymobiledevice3 venv not found. Install with: pipx install pymobiledevice3")
}

func augmentedPath(env []string) []string {
	home, _ := os.UserHomeDir()
	extra := []string{
		filepath.Join(home, ".local", "bin"),
		"/opt/homebrew/bin",
		"/usr/local/bin",
		"/usr/bin",
		"/bin",
	}
	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			env[i] = "PATH=" + strings.Join(extra, ":") + ":" + strings.TrimPrefix(e, "PATH=")
			return env
		}
	}
	return append(env, "PATH="+strings.Join(extra, ":"))
}
