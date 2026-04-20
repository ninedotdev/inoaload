//go:build darwin

package manager

import (
	"context"
	"encoding/json"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/bitxeno/atvloadly/internal/db"
	"github.com/bitxeno/atvloadly/internal/log"
	"github.com/bitxeno/atvloadly/internal/model"
	"github.com/bitxeno/atvloadly/internal/utils"
	gidevice "github.com/electricbubble/gidevice"
	"github.com/grandcat/zeroconf"
)

const (
	mdnsService           = "_apple-mobdev2._tcp"  // legacy iOS/tvOS pairing
	mdnsServiceRemotePair = "_remotepairing._tcp"  // tvOS 17+ / Xcode 15+ pairing
	mdnsServiceDomain     = "local"
)

var usbmux gidevice.Usbmux

func (dm *DeviceManager) Start() {
	dm.mu.Lock()
	dm.ctx, dm.cancel = context.WithCancel(context.Background())
	ctx := dm.ctx
	dm.mu.Unlock()

	umx, err := gidevice.NewUsbmux()
	if err != nil {
		log.Err(err).Msg("Cannot connect to usbmuxd")
		return
	}
	usbmux = umx

	t := time.NewTimer(0)
	defer t.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Info("Device scanner stopped")
				return
			case <-t.C:
				dm.Scan()
				t.Reset(10 * time.Second)
			}
		}
	}()

	<-ctx.Done()
}

func (dm *DeviceManager) Scan() {
	devices, err := usbmux.Devices()
	if err != nil {
		log.Err(err).Msg("Cannot get devices")
		return
	}

	keepConnectedDevices := make(map[string]bool)
	for _, d := range devices {
		if d.Properties().ConnectionType != "Network" {
			continue
		}

		uuid := d.Properties().SerialNumber
		keepConnectedDevices[uuid] = true
		macAddr := strings.Split(d.Properties().EscapedFullServiceName, "@")[0]

		device := model.Device{
			ID:          utils.Md5(uuid),
			Name:        "AppleTV",
			ServiceName: d.Properties().EscapedFullServiceName,
			MacAddr:     macAddr,
			IP:          dm.parseNetworkAddress(d.Properties().NetworkAddress),
			UDID:        uuid,
		}
		if strings.Contains(uuid, ":") {
			device.Status = model.Pairable
		} else {
			res, _ := d.GetValue("", "")
			data, _ := json.Marshal(res)
			devInfo := new(model.UsbmuxdDevice)
			if err := json.Unmarshal(data, devInfo); err == nil {
				device.Name = devInfo.DeviceName
				device.ProductType = devInfo.ProductType
				device.ProductVersion = devInfo.ProductVersion
				device.DeviceClass = devInfo.DeviceClass
			}
			device.Status = model.Paired
			device.ParseDeviceClass()
		}

		dm.devices.Store(uuid, device)
	}

	// Delete non-existent devices
	dm.devices.Range(func(key, value any) bool {
		uuid := key.(string)
		if !keepConnectedDevices[uuid] {
			dm.devices.Delete(uuid)
		}
		return true
	})
}

// data布局：https://github.com/jkcoxson/netmuxd/blob/48494cf6e264bed4e6e1bfa8015767f515ac9ca3/src/devices.rs#L303
func (dm *DeviceManager) parseNetworkAddress(networkAddress []byte) string {
	networkFamily := networkAddress[0]

	if networkFamily == 16 {
		// ipv4
		if ip, ok := netip.AddrFromSlice(networkAddress[4:8]); ok {
			return ip.String()
		}
	}

	if networkFamily == 28 {
		// ipv6
		if ip, ok := netip.AddrFromSlice(networkAddress[16:32]); ok {
			return ip.String()
		}
	}

	return ""

}

func (dm *DeviceManager) ScanServices(ctx context.Context, callback func(serviceType string, name string, host string, address string, port uint16, txt [][]byte)) error {
	return nil
}

func (dm *DeviceManager) ScanWirelessDevices(ctx context.Context, timeout time.Duration) ([]model.Device, error) {
	scanCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var mu sync.Mutex
	var wg sync.WaitGroup
	devices := []model.Device{}

	scan := func(service, defaultClass string) {
		defer wg.Done()
		resolver, err := zeroconf.NewResolver(nil)
		if err != nil {
			return
		}
		entries := make(chan *zeroconf.ServiceEntry)

		done := make(chan struct{})
		go func() {
			defer close(done)
			for entry := range entries {
				serviceName := strings.Replace(entry.Instance, "\\@", "@", -1)
				macAddr := strings.Split(serviceName, "@")[0]
				host := dm.parseName(entry.HostName)

				var ip string
				if len(entry.AddrIPv4) > 0 {
					ip = entry.AddrIPv4[0].String()
				} else if len(entry.AddrIPv6) > 0 {
					ip = entry.AddrIPv6[0].String()
				}
				if ip == "" {
					continue
				}

				// Extract identifier from TXT records (present for _remotepairing._tcp)
				var udid string
				for _, t := range entry.Text {
					if strings.HasPrefix(t, "identifier=") {
						udid = strings.TrimPrefix(t, "identifier=")
						break
					}
				}

				device := model.Device{
					ID:          utils.Md5(serviceName),
					Name:        host,
					ServiceName: serviceName,
					MacAddr:     macAddr,
					IP:          ip,
					UDID:        udid,
					Status:      model.Pairable,
					DeviceClass: defaultClass,
				}
				device.ParseDeviceClass()

				mu.Lock()
				devices = append(devices, device)
				mu.Unlock()
			}
		}()

		if err := resolver.Browse(scanCtx, service, mdnsServiceDomain, entries); err != nil && err != context.DeadlineExceeded {
			log.Err(err).Msgf("mDNS browse failed for %s", service)
		}
		<-done
	}

	wg.Add(2)
	// iOS devices (iPhones/iPads) — classify as iPhone by default (most common)
	go scan(mdnsService, string(model.DeviceClassiPhone))
	// tvOS 17+ devices — always AppleTV
	go scan(mdnsServiceRemotePair, string(model.DeviceClassAppleTV))
	wg.Wait()

	result := dedupeDevices(devices)
	applyPairedRegistry(result)
	return result, nil
}

// applyPairedRegistry marks AppleTV devices as Paired when a prior successful
// RemoteXPC pair for that (name, device_class) was recorded in the local DB.
func applyPairedRegistry(devs []model.Device) {
	store := db.Store()
	if store == nil {
		return
	}
	var records []model.PairedDevice
	if err := store.Find(&records).Error; err != nil {
		return
	}
	byKey := make(map[string]bool, len(records))
	for _, r := range records {
		byKey[strings.ToLower(r.Name)+"/"+strings.ToLower(r.DeviceClass)] = true
	}
	for i := range devs {
		k := strings.ToLower(devs[i].Name) + "/" + strings.ToLower(devs[i].DeviceClass)
		if byKey[k] {
			devs[i].Status = model.Paired
		}
	}
}

// ListPairedDevices returns every persisted pair record.
func ListPairedDevices() []model.PairedDevice {
	store := db.Store()
	if store == nil {
		return nil
	}
	var records []model.PairedDevice
	store.Order("last_paired_at desc").Find(&records)
	return records
}

// DeletePairedDevice removes a persisted pair record. Uses Unscoped so the
// row is physically deleted (preventing unique-index collisions on re-pair
// because GORM's default delete is soft and keeps the row).
func DeletePairedDevice(name, deviceClass string) error {
	store := db.Store()
	if store == nil {
		return nil
	}
	return store.Unscoped().
		Where("name = ? AND device_class = ?", name, deviceClass).
		Delete(&model.PairedDevice{}).Error
}

// SaveRemotePaired records a successful tvOS/iOS 17+ RemoteXPC pair so future
// scans can render the device as already paired. Uses Unscoped so that a
// previously soft-deleted row can be restored/updated instead of colliding
// with the unique (name, device_class) index.
func SaveRemotePaired(name, deviceClass, ip string) {
	store := db.Store()
	if store == nil {
		return
	}
	rec := model.PairedDevice{
		Name:         name,
		DeviceClass:  deviceClass,
		IP:           ip,
		PairType:     "remote",
		LastPairedAt: time.Now(),
	}
	// Clear any previous soft-deleted row under the same (name, class) so we
	// can insert fresh. Then upsert.
	store.Unscoped().
		Where("name = ? AND device_class = ?", name, deviceClass).
		Delete(&model.PairedDevice{})
	store.Create(&rec)
}

func dedupeDevices(in []model.Device) []model.Device {
	// Device identity for deduplication: prefer stable UDID/identifier, else
	// combine name+device_class since a single TV advertises multiple times
	// (IPv4/IPv6/link-local) with varying service-name fragments.
	keyOf := func(d model.Device) string {
		if d.UDID != "" {
			return "u:" + d.UDID
		}
		return "nc:" + strings.ToLower(d.Name) + "/" + strings.ToLower(d.DeviceClass)
	}

	seen := map[string]int{}
	out := make([]model.Device, 0, len(in))
	for _, d := range in {
		key := keyOf(d)
		if idx, ok := seen[key]; ok {
			// Prefer entries with an IPv4 address — they're the easiest to use.
			if strings.Contains(d.IP, ".") && !strings.Contains(out[idx].IP, ".") {
				out[idx] = d
			} else if d.DeviceClass == string(model.DeviceClassAppleTV) && out[idx].DeviceClass != string(model.DeviceClassAppleTV) {
				out[idx] = d
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, d)
	}
	return out
}
