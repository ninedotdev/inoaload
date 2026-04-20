package manager

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/bitxeno/atvloadly/internal/app"
	"github.com/bitxeno/atvloadly/internal/exec"
	"github.com/bitxeno/atvloadly/internal/model"
	"github.com/bitxeno/atvloadly/internal/utils"
)

func StartDeviceManager() {
	deviceManager.Stop()
	go deviceManager.Start()
}

func GetDevices() ([]model.Device, error) {
	return deviceManager.GetDevices(), nil
}

func GetDeviceByID(id string) (*model.Device, bool) {
	return deviceManager.GetDeviceByID(id)
}

func GetDeviceByUDID(udid string) (*model.Device, bool) {
	return deviceManager.GetDeviceByUDID(udid)
}

func ReloadDevices() {
	deviceManager.Scan()
}

func ScanWirelessDevices(ctx context.Context, timeout time.Duration) ([]model.Device, error) {
	return deviceManager.ScanWirelessDevices(ctx, timeout)
}

func ExecuteCommand(name string, args ...string) ([]byte, error) {
	return exec.NewCommand(name, args...).
		WithDir(app.Config.Server.DataDir).
		WithEnv(GetRunEnvs()).
		CombinedOutput()
}

func ExecuteCommandTimeout(timeout time.Duration, name string, args ...string) ([]byte, error) {
	return exec.NewCommand(name, args...).
		WithTimeout(timeout).
		WithDir(app.Config.Server.DataDir).
		WithEnv(GetRunEnvs()).
		CombinedOutput()
}

func GetRunEnvs() []string {
	envs := []string{}
	if app.Settings.Network.ProxyEnabled {
		if app.Settings.Network.HTTPProxy != "" {
			envs = append(envs, fmt.Sprintf("HTTP_PROXY=%s", app.Settings.Network.HTTPProxy))
			envs = append(envs, fmt.Sprintf("http_proxy=%s", app.Settings.Network.HTTPProxy))
		}
		if app.Settings.Network.HTTPSProxy != "" {
			envs = append(envs, fmt.Sprintf("HTTPS_PROXY=%s", app.Settings.Network.HTTPSProxy))
			envs = append(envs, fmt.Sprintf("https_proxy=%s", app.Settings.Network.HTTPSProxy))
		}
	}
	return utils.MergeEnvs(os.Environ(), envs)
}
