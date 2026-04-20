# inoaload

Native macOS app for sideloading `.ipa` files to **Apple TV on tvOS 17+**. Built
with Wails v2 (Go + React 19 + Vite 8 + shadcn/ui). Free-tier Apple ID signing
works — no paid Developer Program required.

This project reuses the domain logic from
[bitxeno/atvloadly](https://github.com/bitxeno/atvloadly) (Apple ID login,
certificate management, install queue) and wraps it in a new Mac-native shell
with a fresh UI and an additional tvOS 17+ pipeline. Not a fork — a standalone
app with credits (see bottom) to the upstream projects whose work made this
possible.

## Why this exists

`atvloadly` only supports Linux with the legacy `_apple-mobdev2._tcp` pairing
protocol. tvOS 17+ abandoned that in favour of `_remotepairing._tcp` (RemoteXPC,
Xcode 15+). This project adds:

- Native macOS build (no Docker)
- Discovery + pair for tvOS 17+ via `pymobiledevice3`
- IPA push via RemoteXPC tunnel (`tunneld`)
- Embedded `plumesign` Rust binary for Apple ID signing
- Patched `omnisette` + `plumesign` to survive Apple API drift (`EndProvisioningError` variant, team-id override)

## Requirements

- macOS 14+ on Apple Silicon (tested on arm64)
- `libimobiledevice` (`brew install libimobiledevice`) — only used for legacy iOS devices
- `pipx install pymobiledevice3` — required for tvOS 17+ pairing and push
- Admin password (once) to start the RemoteXPC tunnel daemon

## Install & run

1. Download the `.app` from Releases (or build from source — see below).
2. Install prerequisites:
   ```sh
   brew install libimobiledevice pipx
   pipx install pymobiledevice3
   ```
3. Launch the app. On first run macOS will prompt for **Local Network** access — allow it.
4. In **Settings**:
   - Save your Apple ID email + password.
   - Click **Load teams**, then pick your personal (Individual) team.
   - Click **Start (admin)** under _tvOS Install Tunnel_ and authorise — this starts `pymobiledevice3 remote tunneld` as root.
5. In **Devices**:
   - Put the Apple TV in _Settings → Remotes and Devices → Remote App and Devices_.
   - Click **Deep scan** if needed. Click **Pair…** on the TV card. Enter the 6-digit PIN when prompted.
6. In **Install**: drop an `.ipa`. The app logs in with 2FA, registers the device, signs with `plumesign` and pushes via RemoteXPC. ~1 minute end to end.

Signed apps expire after 7 days — re-run install to refresh (automatic re-sign is on the roadmap).

## Architecture

```
┌──────────────────┐       ┌──────────────────┐      ┌─────────────┐
│ React UI (Vite)  │──WS──▶│  Fiber backend   │─────▶│  plumesign  │  sign
│ shadcn/ui        │◀──────│  (Go, embedded)  │      │  (embedded) │
└──────────────────┘       │                  │      └─────────────┘
         ▲                 │                  │      ┌─────────────┐
         │  wails://       │                  │─────▶│pymobiledev3 │  pair/push
         └─────────────────│                  │      │ (subprocess)│
                           └──────────────────┘      └─────────────┘
                                   │                        │
                                   ▼                        ▼
                              SQLite app.db            tunneld :49151
                              (paired_devices)         (root-spawned)
```

- **Frontend** (`frontend/src/App.tsx`): React 19 + Vite 8 + Tailwind v4 +
  shadcn/ui + Hugeicons. Drop zone via `react-dropzone`, OTP via
  `shadcn/input-otp`.
- **Wails shell**: WKWebView that serves the embedded React build and
  reverse-proxies `/api/*` to the embedded Fiber backend on
  `127.0.0.1:6066`. WebSockets connect to Fiber directly (Wails' asset-server
  proxy doesn't forward HTTP upgrades).
- **Go backend**:
  - `internal/manager/device_manager_usbmuxd.go` — parallel mDNS browse of
    `_apple-mobdev2._tcp` and `_remotepairing._tcp`, plus usbmuxd for legacy.
  - `internal/manager/pair_manager_remote.go` + `pair_helper.py` — pairing for
    tvOS 17+ via `pymobiledevice3.remote.tunnel_service.RemotePairingManualPairingService`.
  - `internal/manager/install_manager_remote.go` — three-phase pipeline
    (register device → `plumesign sign` → `pymobiledevice3 apps install`).
  - `internal/manager/tunneld_manager.go` — spawns `pymobiledevice3 remote
tunneld` via `osascript do shell script with administrator privileges` so
    the user gets the native macOS prompt.
  - `internal/manager/udid_resolver_darwin.go` — translates the rotating
    RemoteXPC mDNS identifier to the stable hardware UDID by querying tunneld
    and `lockdown info`. Falls back to matching `DeviceName` across tunnels
    when the identifier has rotated mid-operation.
  - `internal/manager/plumesign_bin_darwin.go` — `//go:embed bin/plumesign`
    extracted to `$TMPDIR/wails-sideload/plumesign` at boot; `$PATH` is
    augmented so any `exec.Command("plumesign", …)` resolves to the bundled
    copy.
- **DB** (`internal/model/paired_device.go`): SQLite via gorm. Persists
  `PairedDevice` rows since `pymobiledevice3`'s pair records only contain
  crypto keys with no device identity.

### Patched upstream dependencies

These live outside the repo at `/tmp/omnisette` and `/tmp/PlumeImpactor` during
build:

- **omnisette**: `remote_anisette_v3.rs` — adds `EndProvisioningError` variant
  to `ProvisionInput` enum so Apple's newer response doesn't crash the parser
  on unknown variants.
- **plumesign**: `commands/account.rs teams()` — honours `PLUMESIGN_TEAM_ID`
  env var before auto-picking; prefers `xcode_free_only` teams over paid ones
  when no override is set.

## Build from source

```sh
# toolchain
brew install go node pipx rust
pipx install pymobiledevice3

# plumesign binary (Rust, ~100s cold build)
git clone --branch v2.2.3-patch.1 https://github.com/bitxeno/PlumeImpactor /tmp/PlumeImpactor
git clone --branch patch           https://github.com/bitxeno/omnisette     /tmp/omnisette
# Apply the two patches described above. Then:
cd /tmp/PlumeImpactor
cat >> Cargo.toml <<'EOF'
[patch."https://github.com/bitxeno/omnisette"]
omnisette = { path = "/tmp/omnisette/omnisette" }
EOF
cargo build --bin plumesign
cp target/debug/plumesign /path/to/wails-sideload/internal/manager/bin/plumesign

# app
cd /path/to/wails-sideload/frontend && npm install && npm run build
cd .. && wails build -platform darwin/arm64
```

Output: `build/bin/wails-sideload.app` (~104 MB with the embedded plumesign).
Launch with `open build/bin/wails-sideload.app`.

## Known limitations

- tvOS 16 or earlier is untested — the legacy code path is still in the tree
  but not exercised here.
- No automatic re-sign before the 7-day certificate expiry yet (planned).
- The embedded `plumesign` is a debug build (~80 MB). A release LTO build
  would shrink it to ~15 MB but takes much longer to compile.
- Apple Local Network permission must be granted once on first launch
  (declared via `NSBonjourServices` + `NSLocalNetworkUsageDescription` in
  `build/darwin/Info.plist`).

## Troubleshooting

- **`plumesign: command not found`** — the embedded binary didn't extract.
  Quit and re-open the app; check `$TMPDIR/wails-sideload/plumesign` exists.
- **`Developer API error 4550 … Program License Agreement`** — your Apple ID
  has an unresolved team PLA. Go to <https://developer.apple.com/account>,
  either accept the PLA banner or _Leave team_ for any team where you're only
  a member.
- **`An invalid value '…' was provided for the parameter 'deviceNumber'`** —
  the mDNS identifier was sent where Apple expects a hardware UDID.
  `ResolveTunnel` normally catches this; ensure tunneld is running.
- **`Device not found: <uuid>`** from `pymobiledevice3 apps install` —
  `--tunnel` expects the hardware UDID, not the mDNS identifier. Fixed in
  `install_manager_remote.go` — re-install the app if you're seeing this on
  an old build.
- **`tunneld has no tunnel for … after 25s`** — the TV is asleep or the
  identifier rotated. Wake it up, make sure _Remote App and Devices_ is open.

## Credits

Released under MIT. This project stands on the shoulders of:

- [bitxeno/atvloadly](https://github.com/bitxeno/atvloadly)
- [claration/Impactor](https://github.com/claration/Impactor)
- [doronz88/pymobiledevice3](https://github.com/doronz88/pymobiledevice3)
- [libimobiledevice](https://github.com/libimobiledevice)
