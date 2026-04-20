package manager

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bitxeno/atvloadly/internal/app"
	"github.com/bitxeno/atvloadly/internal/log"
	"github.com/bitxeno/atvloadly/internal/utils"
)

// ErrAccountInvalid is returned when plumesign signals the Apple ID login
// cannot be used for signing (bad password, no valid session, etc.).
var ErrAccountInvalid = errors.New("account invalid")

// InstallOptions captures the parameters passed down to the RemoteInstallManager.
type InstallOptions struct {
	UDID             string
	Account          string
	Password         string
	IpaPath          string
	RemoveExtensions bool
	TeamID           string
	DeviceName       string
}

// TaskLogger accumulates every line emitted during a remote install so the
// full transcript can be persisted as the per-task log file surfaced by the UI.
type TaskLogger struct {
	mu  sync.Mutex
	buf strings.Builder
}

func NewTaskLogger() *TaskLogger { return &TaskLogger{} }

func (l *TaskLogger) Write(line string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf.WriteString(line)
	if !strings.HasSuffix(line, "\n") {
		l.buf.WriteString("\n")
	}
}

func (l *TaskLogger) Output() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

func (l *TaskLogger) IsSuccess() bool {
	o := l.Output()
	return strings.Contains(o, "Installation Succeeded") ||
		strings.Contains(o, "Installation complete") ||
		strings.Contains(o, "Installation succeed")
}

func (l *TaskLogger) IsAccountInvalid() bool {
	o := l.Output()
	return strings.Contains(o, "Can't log-in") ||
		strings.Contains(o, "DeveloperSession creation failed")
}

func (l *TaskLogger) SaveLog(id uint) {
	saveDir := filepath.Join(app.Config.Server.DataDir, "log")
	if err := os.MkdirAll(saveDir, os.ModePerm); err != nil {
		log.Error("failed to create log dir: " + saveDir)
		return
	}
	path := filepath.Join(saveDir, fmt.Sprintf("task_%d.log", id))
	_ = os.WriteFile(path, []byte(l.Output()), 0644)
}

// CleanInstallTempFiles removes any intermediate IPA/icon/plumesign staging
// artifacts left after an install finishes.
func CleanInstallTempFiles(ipaPath string) {
	if ipaPath == "" {
		return
	}
	ipaName := filepath.Base(ipaPath)
	base := strings.TrimSuffix(ipaName, filepath.Ext(ipaName))

	utils.RemoveAllFiles(filepath.Join(app.Config.Server.DataDir, "tmp"), base+"*")
	utils.RemoveAllFiles(os.TempDir(), base+"*")
	utils.RemoveAllFiles(os.TempDir(), "plume_stage*")
}
