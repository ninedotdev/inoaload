package manager

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/bitxeno/atvloadly/internal/model"
)

var deviceManager = newDeviceManager()

type DeviceManager struct {
	devices sync.Map
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
}

func newDeviceManager() *DeviceManager {
	return &DeviceManager{}
}

func (dm *DeviceManager) GetDevices() []model.Device {
	devices := []model.Device{}
	dm.devices.Range(func(k, v any) bool {
		devices = append(devices, v.(model.Device))
		return true
	})

	// Sort devices: AppleTV first, then by DeviceClass, then by Name.
	sort.Slice(devices, func(i, j int) bool {
		ci, cj := strings.ToLower(devices[i].DeviceClass), strings.ToLower(devices[j].DeviceClass)
		isATVi := strings.Contains(ci, "appletv")
		isATVj := strings.Contains(cj, "appletv")
		if isATVi != isATVj {
			return isATVi
		}
		if ci != cj {
			return ci < cj
		}
		return devices[i].Name < devices[j].Name
	})

	return devices
}

func (dm *DeviceManager) GetDeviceByID(id string) (*model.Device, bool) {
	for _, d := range dm.GetDevices() {
		if d.ID == id {
			return &d, true
		}
	}
	return nil, false
}

func (dm *DeviceManager) GetDeviceByUDID(udid string) (*model.Device, bool) {
	for _, d := range dm.GetDevices() {
		if d.UDID == udid {
			return &d, true
		}
	}
	return nil, false
}

func (dm *DeviceManager) SaveDevice(dev model.Device) {
	dm.devices.Store(dev.UDID, dev)
}

func (dm *DeviceManager) DeleteDevice(udid string) {
	dm.devices.Delete(udid)
}

func (dm *DeviceManager) parseName(host string) string {
	name := strings.TrimSuffix(host, ".")
	return strings.TrimSuffix(name, ".local")
}

func (dm *DeviceManager) Stop() {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	if dm.cancel != nil {
		dm.cancel()
		dm.cancel = nil
		dm.ctx = nil
	}
}
