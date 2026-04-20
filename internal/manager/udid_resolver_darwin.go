//go:build darwin

package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// tunneldEntry represents one tunnel record returned by tunneld's HTTP index.
type tunneldEntry struct {
	Address   string `json:"tunnel-address"`
	Port      int    `json:"tunnel-port"`
	Interface string `json:"interface"`
}

// ResolveHardwareUDID translates a RemoteXPC advertised identifier (a UUID
// seen in mDNS TXT records) to the legacy Apple hardware UDID (e.g.
// `00008110-000C143E3E0A201E`). The translation is:
//
//	1. Ask tunneld which address:port its tunnel to this identifier listens on.
//	2. Ask lockdownd (over that tunnel) for UniqueDeviceID.
//
// Falls back to the input identifier if the translation can't be performed.
// ResolveHardwareUDIDByName also accepts a device name to match when the
// mdnsID has rotated (tvOS identifiers change across pair-mode transitions).
func ResolveHardwareUDID(ctx context.Context, mdnsID string) (string, error) {
	return ResolveHardwareUDIDByName(ctx, mdnsID, "")
}

// TunnelResolution bundles the hardware UDID + the tunneld key that currently
// maps to the device (which may differ from the originally-advertised mDNS id
// because tvOS rotates the remote pairing identifier).
type TunnelResolution struct {
	HardwareUDID string
	TunnelKey    string
}

func ResolveHardwareUDIDByName(ctx context.Context, mdnsID, name string) (string, error) {
	r, err := ResolveTunnel(ctx, mdnsID, name)
	if err != nil {
		return "", err
	}
	return r.HardwareUDID, nil
}

func ResolveTunnel(ctx context.Context, mdnsID, name string) (TunnelResolution, error) {
	if !IsTunneldRunning() {
		return TunnelResolution{}, fmt.Errorf("tunneld is not running")
	}

	var entry tunneldEntry
	var key string
	deadline := time.Now().Add(25 * time.Second)
	for {
		tunnels, err := fetchTunneldIndex(ctx)
		if err == nil {
			if entries, ok := tunnels[mdnsID]; ok && len(entries) > 0 {
				entry = entries[0]
				key = mdnsID
				break
			}
			if name != "" {
				if e, k, ok := findTunnelByNameWithKey(ctx, tunnels, name); ok {
					entry = e
					key = k
					break
				}
			}
		}
		if time.Now().After(deadline) {
			return TunnelResolution{}, fmt.Errorf("tunneld has no tunnel for %q after 25s — is the TV awake?", name)
		}
		select {
		case <-ctx.Done():
			return TunnelResolution{}, ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}

	py, err := resolvePipxPython()
	if err != nil {
		return TunnelResolution{}, err
	}
	cli := strings.Replace(py, "/bin/python", "/bin/pymobiledevice3", 1)

	lockCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(lockCtx, cli, "lockdown", "info",
		"--rsd", entry.Address, fmt.Sprintf("%d", entry.Port))
	cmd.Env = augmentedPath(os.Environ())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return TunnelResolution{}, fmt.Errorf("lockdown info failed: %s", strings.TrimSpace(string(out)))
	}

	re := regexp.MustCompile(`"UniqueDeviceID"\s*:\s*"([^"]+)"`)
	m := re.FindStringSubmatch(string(out))
	if len(m) != 2 {
		return TunnelResolution{}, fmt.Errorf("UniqueDeviceID not found in lockdown output")
	}
	return TunnelResolution{HardwareUDID: m[1], TunnelKey: key}, nil
}

// findTunnelByNameWithKey iterates every known tunnel, asks lockdown for
// DeviceName, and returns the matching entry + its tunneld key.
func findTunnelByNameWithKey(ctx context.Context, tunnels map[string][]tunneldEntry, targetName string) (tunneldEntry, string, bool) {
	py, err := resolvePipxPython()
	if err != nil {
		return tunneldEntry{}, "", false
	}
	cli := strings.Replace(py, "/bin/python", "/bin/pymobiledevice3", 1)

	for key, entries := range tunnels {
		for _, e := range entries {
			lockCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			cmd := exec.CommandContext(lockCtx, cli, "lockdown", "info",
				"--rsd", e.Address, fmt.Sprintf("%d", e.Port))
			cmd.Env = augmentedPath(os.Environ())
			out, err := cmd.CombinedOutput()
			cancel()
			if err != nil {
				continue
			}
			nameRe := regexp.MustCompile(`"DeviceName"\s*:\s*"([^"]+)"`)
			if m := nameRe.FindStringSubmatch(string(out)); len(m) == 2 && strings.EqualFold(m[1], targetName) {
				return e, key, true
			}
		}
	}
	return tunneldEntry{}, "", false
}

func fetchTunneldIndex(ctx context.Context) (map[string][]tunneldEntry, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("http://%s:%d/", tunneldHost, tunneldPort), nil)
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var parsed map[string][]tunneldEntry
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}
