import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useDropzone } from 'react-dropzone'
import { toast } from 'sonner'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  AntennaIcon,
  PackageAddIcon,
  ComputerTerminalIcon,
  Tv01Icon,
  SmartPhone01Icon,
  Tablet01Icon,
  RefreshIcon,
  CloudUploadIcon,
  AppleIcon,
  RadarIcon,
  InformationCircleIcon,
  Copy01Icon,
  Settings01Icon,
  DashboardSquare01Icon,
  Delete02Icon,
  PackageReceiveIcon,
} from '@hugeicons/core-free-icons'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { InputOTP, InputOTPGroup, InputOTPSlot } from '@/components/ui/input-otp'
import { Toaster } from '@/components/ui/sonner'

type Device = {
  id: string
  name: string
  ip: string
  udid: string
  mac_addr: string
  status: string
  product_type?: string
  product_version?: string
  device_class?: string
}

type ApiResult<T> = { code: number; msg: string; data: T }

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init)
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  const json = (await res.json()) as ApiResult<T>
  if (json.code !== 200 && json.code !== 0) throw new Error(json.msg || 'API error')
  return json.data
}

type Tab = 'devices' | 'install' | 'apps' | 'logs' | 'settings'

const nav: { id: Tab; label: string; icon: typeof AntennaIcon }[] = [
  { id: 'devices', label: 'Devices', icon: AntennaIcon },
  { id: 'install', label: 'Install', icon: PackageAddIcon },
  { id: 'apps', label: 'Apps', icon: DashboardSquare01Icon },
  { id: 'logs', label: 'Logs', icon: ComputerTerminalIcon },
  { id: 'settings', label: 'Settings', icon: Settings01Icon },
]

const SETTINGS_KEY = 'wails-sideload:apple_id'

type AppleIDSettings = { email: string; password: string; team_id?: string }

function loadAppleID(): AppleIDSettings {
  try {
    const raw = localStorage.getItem(SETTINGS_KEY)
    if (!raw) return { email: '', password: '' }
    return JSON.parse(raw)
  } catch {
    return { email: '', password: '' }
  }
}

function saveAppleID(v: AppleIDSettings) {
  localStorage.setItem(SETTINGS_KEY, JSON.stringify(v))
}

type AppleTeam = { id: string; name: string; type?: string; free_tier?: boolean }

function iconForDevice(d: Device) {
  const c = (d.device_class || '').toLowerCase()
  if (c === 'appletv') return Tv01Icon
  if (c === 'ipad') return Tablet01Icon
  if (c === 'iphone') return SmartPhone01Icon
  return AppleIcon
}

function Sidebar({ tab, setTab, deviceCount, tvCount }: { tab: Tab; setTab: (t: Tab) => void; deviceCount: number; tvCount: number }) {
  return (
    <aside className="w-[220px] shrink-0 flex flex-col">
      {/* Spacer for traffic lights — draggable */}
      <div className="drag h-[52px]" />

      <nav className="no-drag px-2 py-1 flex-1 space-y-0.5">
        {nav.map((n) => {
          const active = tab === n.id
          const count = n.id === 'devices' ? deviceCount : n.id === 'install' ? tvCount : 0
          return (
            <button
              key={n.id}
              onClick={() => setTab(n.id)}
              className={`w-full flex items-center gap-2.5 px-2.5 py-1.5 rounded-md text-[13px] transition-colors ${
                active
                  ? 'bg-accent text-accent-foreground font-medium'
                  : 'text-muted-foreground hover:bg-accent/40 hover:text-foreground'
              }`}
            >
              <HugeiconsIcon icon={n.icon} size={16} strokeWidth={1.8} />
              <span>{n.label}</span>
              {count > 0 && (
                <span className="ml-auto text-[10px] font-mono bg-muted/80 text-muted-foreground rounded px-1.5 py-0.5">
                  {count}
                </span>
              )}
            </button>
          )
        })}
      </nav>

      <div className="no-drag p-3 text-[10px] text-muted-foreground/60">
        <div className="font-mono tracking-tight">iNoaload</div>
        <div>macOS · v0.1</div>
      </div>
    </aside>
  )
}

function DeviceCard({ d, onPair, onUnpair }: { d: Device; onPair?: (d: Device) => void; onUnpair?: (d: Device) => void }) {
  const isPaired = d.status === 'paired'
  const isAppleTV = (d.device_class || '').toLowerCase() === 'appletv'

  return (
    <div className="group rounded-lg border border-border/60 bg-card/40 hover:bg-card/60 backdrop-blur-sm transition-colors">
      <div className="flex items-start gap-3 p-3.5">
        <div className="shrink-0 h-9 w-9 rounded-md bg-muted/60 text-muted-foreground flex items-center justify-center">
          <HugeiconsIcon icon={iconForDevice(d)} size={18} strokeWidth={1.6} />
        </div>

        <div className="min-w-0 flex-1">
          <div className="text-[13px] font-medium truncate">{d.name}</div>
          <div className="mt-0.5 text-[11px] text-muted-foreground truncate">
            {[
              d.ip || null,
              isAppleTV ? (d.product_version ? `tvOS ${d.product_version}` : 'Apple TV') : d.product_type || d.device_class,
              d.product_version && !isAppleTV ? d.product_version : null,
            ]
              .filter(Boolean)
              .join(' · ')}
          </div>
        </div>

        <div className="shrink-0 flex items-center gap-2">
          {isPaired ? (
            <>
              <span className="inline-flex items-center gap-1 text-[11px] text-muted-foreground">
                <span className="h-1.5 w-1.5 rounded-full bg-emerald-500/90" />
                Paired
              </span>
              {onUnpair && (
                <Button
                  size="sm"
                  variant="ghost"
                  className="no-drag h-7 text-[11px] px-2 opacity-0 group-hover:opacity-100 transition-opacity text-muted-foreground hover:text-destructive"
                  onClick={() => onUnpair(d)}
                >
                  Unpair
                </Button>
              )}
            </>
          ) : (
            <Button
              size="sm"
              variant="secondary"
              className="no-drag h-7 text-[11px] px-2.5"
              onClick={() => onPair?.(d)}
            >
              Pair…
            </Button>
          )}
        </div>
      </div>
    </div>
  )
}

function DevicesView({
  devices,
  loading,
  onRefresh,
  onPair,
  onUnpair,
}: {
  devices: Device[]
  loading: boolean
  onRefresh: () => void
  onPair: (d: Device) => void
  onUnpair: (d: Device) => void
}) {
  const [deepScanning, setDeepScanning] = useState(false)

  const tvs = devices.filter((d) => (d.device_class || '').toLowerCase() === 'appletv')
  const others = devices.filter((d) => (d.device_class || '').toLowerCase() !== 'appletv')

  const deepScan = async () => {
    setDeepScanning(true)
    try {
      const found = await api<Device[]>('/api/scan/wireless?timeout=5')
      toast.success(`Found ${found?.length ?? 0} device${found?.length === 1 ? '' : 's'} in deep scan`)
      onRefresh()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Deep scan failed')
    } finally {
      setDeepScanning(false)
    }
  }

  return (
    <div className="space-y-7">
      <ViewHeader
        title="Devices"
        subtitle="Paired and discoverable Apple devices on this network"
        actions={
          <div className="flex items-center gap-2">
            <Button variant="ghost" size="sm" onClick={deepScan} disabled={deepScanning} className="no-drag gap-1.5 text-muted-foreground hover:text-foreground">
              <HugeiconsIcon icon={RadarIcon} size={14} strokeWidth={1.8} className={deepScanning ? 'animate-pulse' : ''} />
              {deepScanning ? 'Scanning network…' : 'Deep scan'}
            </Button>
            <Button variant="outline" size="sm" onClick={onRefresh} disabled={loading} className="no-drag gap-1.5">
              <HugeiconsIcon icon={RefreshIcon} size={14} strokeWidth={1.8} className={loading ? 'animate-spin' : ''} />
              Refresh
            </Button>
          </div>
        }
      />

      {tvs.length === 0 && (
        <AppleTvOnboarding />
      )}

      {tvs.length > 0 && (
        <DeviceGroup title="Apple TV" badge={`${tvs.length}`} devices={tvs} onPair={onPair} onUnpair={onUnpair} />
      )}

      {others.length > 0 && (
        <DeviceGroup title="Other Apple devices" badge={`${others.length}`} devices={others} onPair={onPair} onUnpair={onUnpair} />
      )}

      {devices.length === 0 && (
        <EmptyState
          title="No devices detected"
          hint="Connect a device via USB-C or enable Remote App and Devices on your Apple TV."
        />
      )}
    </div>
  )
}

function AppleTvOnboarding() {
  return (
    <div className="rounded-xl border border-border/60 bg-card/40 backdrop-blur-sm p-5 flex gap-4">
      <div className="shrink-0 h-10 w-10 rounded-lg bg-primary/10 text-primary flex items-center justify-center">
        <HugeiconsIcon icon={Tv01Icon} size={20} strokeWidth={1.6} />
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <h3 className="text-[13px] font-semibold">Waiting for Apple TV</h3>
          <span className="text-[9px] font-mono uppercase tracking-wider text-primary/70 bg-primary/10 rounded px-1 py-0.5">
            tvOS
          </span>
        </div>
        <p className="text-[12px] text-muted-foreground mt-1">
          Apple TVs only show up in pairing mode. On tvOS:
        </p>
        <ol className="mt-2 text-[12px] text-muted-foreground space-y-1 list-decimal list-inside marker:text-muted-foreground/60">
          <li>
            Open <span className="text-foreground">Settings → Remotes and Devices → Remote App and Devices</span>
          </li>
          <li>
            Keep that screen open — it advertises the pairing service over mDNS
          </li>
          <li>
            Make sure your Apple TV and this Mac are on the same network
          </li>
        </ol>
        <p className="mt-3 text-[11px] text-muted-foreground/70 flex items-center gap-1.5">
          <HugeiconsIcon icon={InformationCircleIcon} size={12} strokeWidth={1.8} />
          tvOS 17+ also needs Developer Mode enabled for sideloading.
        </p>
      </div>
    </div>
  )
}

function DeviceGroup({
  title,
  badge,
  devices,
  emptyHint,
  onPair,
  onUnpair,
}: {
  title: string
  badge?: string
  devices: Device[]
  emptyHint?: string
  onPair?: (d: Device) => void
  onUnpair?: (d: Device) => void
}) {
  return (
    <section className="space-y-3">
      <div className="flex items-center gap-2">
        <h2 className="text-[11px] font-semibold uppercase tracking-[0.1em] text-muted-foreground">{title}</h2>
        {badge && <span className="text-[10px] font-mono text-muted-foreground/60">{badge}</span>}
      </div>
      {devices.length === 0 ? (
        emptyHint && <EmptyState title={emptyHint} compact />
      ) : (
        <div className="grid grid-cols-1 xl:grid-cols-2 gap-2.5">
          {devices.map((d) => (
            <DeviceCard key={d.udid || d.id} d={d} onPair={onPair} onUnpair={onUnpair} />
          ))}
        </div>
      )}
    </section>
  )
}

type PairPhase = 'idle' | 'running' | 'awaiting_pin' | 'success' | 'error'

function PairDialog({
  device,
  onClose,
  onPaired,
}: {
  device: Device | null
  onClose: () => void
  onPaired: () => void
}) {
  const [log, setLog] = useState<string[]>([])
  const [code, setCode] = useState('')
  const [phase, setPhase] = useState<PairPhase>('idle')
  const wsRef = useRef<WebSocket | null>(null)
  const logEndRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    // reset when dialog reopens
    if (device) {
      setLog([])
      setCode('')
      setPhase('idle')
    }
    return () => {
      wsRef.current?.close()
      wsRef.current = null
    }
  }, [device])

  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: 'smooth', block: 'end' })
  }, [log])

  const startPair = () => {
    if (!device) return
    // Connect directly to the embedded Fiber server. Wails' AssetServer reverse
    // proxy does not support WebSocket upgrades (no Hijacker on its ResponseWriter),
    // so we bypass it for /ws/* routes.
    const ws = new WebSocket(`ws://127.0.0.1:6066/ws/pair`)
    wsRef.current = ws
    setPhase('running')
    // Apple TVs always use the RemoteXPC protocol (tvOS 17+ abandoned legacy
    // _apple-mobdev2 pairing). Everything else uses the classic idevicepair flow.
    const useRemote = (device.device_class || '').toLowerCase() === 'appletv'
    const payload = useRemote ? `remote:${device.name}` : device.udid
    setLog((l) => [
      ...l,
      `→ Pairing ${device.name} via ${useRemote ? 'pymobiledevice3 (RemoteXPC)' : 'idevicepair (legacy)'}`,
    ])

    ws.onopen = () => {
      ws.send(JSON.stringify({ t: 1, d: payload }))
      setLog((l) => [...l, '→ Accept the trust prompt on the device when it appears.'])
    }
    ws.onmessage = (ev) => {
      const line = String(ev.data)
      setLog((l) => [...l, line])
      const lower = line.toLowerCase()
      if (lower.startsWith('success:')) setPhase('success')
      else if (lower.startsWith('error:')) setPhase('error')
      else if (lower.startsWith('enter pin')) setPhase('awaiting_pin')
    }
    ws.onerror = () => setPhase('error')
    // Do not auto-transition to error on close — keep the phase set by explicit
    // SUCCESS/ERROR markers from the backend.
  }

  const submitCode = (value?: string) => {
    const pin = (value ?? code).trim()
    if (!pin || !wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) return
    wsRef.current.send(JSON.stringify({ t: 2, d: pin }))
    setLog((l) => [...l, `→ Submitted PIN ${pin}`])
    setCode('')
    setPhase('running')
  }

  const close = () => {
    wsRef.current?.close()
    if (phase === 'success') onPaired()
    onClose()
  }

  return (
    <Dialog open={!!device} onOpenChange={(o) => !o && close()}>
      <DialogContent className="sm:max-w-[520px]">
        <DialogHeader>
          <DialogTitle>Pair {device?.name || 'device'}</DialogTitle>
          <DialogDescription>
            Open <span className="font-medium text-foreground">Settings → Remotes and Devices → Remote App and Devices</span> on the Apple TV and keep that screen visible during pairing.
          </DialogDescription>
        </DialogHeader>

        {device && (
          <div className="space-y-3 text-[13px]">
            <div className="rounded-md border border-border/60 bg-muted/30 p-3 space-y-1.5">
              <InfoRow label="IP" value={device.ip || '—'} copyable />
              <InfoRow label="UDID" value={device.udid || '—'} copyable mono />
            </div>

            {phase === 'idle' ? (
              <div className="flex justify-end">
                <Button size="sm" onClick={startPair} disabled={!device.udid && !device.name}>
                  Start pairing
                </Button>
              </div>
            ) : (
              <>
                <div className="rounded-md border border-border/60 bg-black/20 p-3 font-mono text-[11px] max-h-[180px] overflow-auto space-y-0.5">
                  {log.map((l, i) => (
                    <div key={i} className="whitespace-pre-wrap break-all text-muted-foreground">
                      {l}
                    </div>
                  ))}
                  <div ref={logEndRef} />
                </div>

                {phase === 'awaiting_pin' && (
                  <div className="no-drag space-y-3 py-2">
                    <div className="text-[12px] text-muted-foreground text-center">
                      Enter the 6-digit code shown on the Apple TV
                    </div>
                    <div className="flex justify-center">
                      <InputOTP
                        maxLength={6}
                        value={code}
                        onChange={(v) => {
                          setCode(v)
                          if (v.length === 6) submitCode(v)
                        }}
                        autoFocus
                      >
                        <InputOTPGroup>
                          <InputOTPSlot index={0} />
                          <InputOTPSlot index={1} />
                          <InputOTPSlot index={2} />
                          <InputOTPSlot index={3} />
                          <InputOTPSlot index={4} />
                          <InputOTPSlot index={5} />
                        </InputOTPGroup>
                      </InputOTP>
                    </div>
                  </div>
                )}

                {phase === 'success' && (
                  <div className="text-[12px] text-emerald-500">✓ Pairing completed — you can close this dialog.</div>
                )}
                {phase === 'error' && (
                  <div className="text-[12px] text-destructive">Pairing failed. Check the log above and try again.</div>
                )}
              </>
            )}
          </div>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={close}>
            Close
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function InfoRow({
  label,
  value,
  copyable,
  mono,
}: {
  label: string
  value: string
  copyable?: boolean
  mono?: boolean
}) {
  return (
    <div className="flex items-start gap-3 text-[12px]">
      <span className="w-16 shrink-0 text-muted-foreground">{label}</span>
      <span className={`flex-1 min-w-0 truncate ${mono ? 'font-mono text-[11px]' : ''}`}>{value}</span>
      {copyable && (
        <button
          className="shrink-0 text-muted-foreground hover:text-foreground transition-colors"
          onClick={() => {
            navigator.clipboard.writeText(value)
            toast.success(`Copied ${label.toLowerCase()}`)
          }}
          aria-label={`Copy ${label}`}
        >
          <HugeiconsIcon icon={Copy01Icon} size={13} strokeWidth={1.8} />
        </button>
      )}
    </div>
  )
}

type InstallPhase = 'idle' | 'uploading' | 'installing' | 'awaiting_2fa' | 'success' | 'error'

function InstallView({ devices }: { devices: Device[] }) {
  // Install targets: any paired device (Apple TV, iPhone, iPad).
  const tvs = useMemo(
    () =>
      devices.filter((d) => {
        if (d.status !== 'paired') return false
        const c = (d.device_class || '').toLowerCase()
        return c === 'appletv' || c === 'iphone' || c === 'ipad'
      }),
    [devices],
  )
  const [selected, setSelected] = useState<string>('')
  const [{ email: account, password }, setCreds] = useState(loadAppleID())
  const [phase, setPhase] = useState<InstallPhase>('idle')
  // Refresh credentials when returning to this tab.
  useEffect(() => {
    const onFocus = () => setCreds(loadAppleID())
    window.addEventListener('focus', onFocus)
    const iv = setInterval(onFocus, 1500)
    return () => {
      window.removeEventListener('focus', onFocus)
      clearInterval(iv)
    }
  }, [])
  const [log, setLog] = useState<string[]>([])
  const [otp, setOtp] = useState('')
  const wsRef = useRef<WebSocket | null>(null)
  const logEndRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (!selected && tvs.length > 0) setSelected(tvs[0].id)
  }, [tvs, selected])

  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: 'smooth', block: 'end' })
  }, [log])

  useEffect(() => () => wsRef.current?.close(), [])

  const selectedDevice = tvs.find((d) => d.id === selected)

  const startInstall = (
    ipaPath: string,
    udid: string,
    ipaMeta?: { name: string; bundle: string; version: string; icon: string },
  ) => {
    const ws = new WebSocket(`ws://127.0.0.1:6066/ws/install`)
    wsRef.current = ws
    setPhase('installing')
    ws.onopen = () => {
      const creds = loadAppleID()
      const data = JSON.stringify({
        ipa_path: ipaPath,
        ipa_name: ipaMeta?.name || '',
        bundle_identifier: ipaMeta?.bundle || '',
        version: ipaMeta?.version || '',
        icon: ipaMeta?.icon || '',
        udid,
        device: selectedDevice?.name || '',
        account,
        password,
        remove_extensions: false,
        team_id: creds.team_id || '',
      })
      ws.send(JSON.stringify({ t: 1, d: data }))
      setLog((l) => [...l, '→ Install started on backend'])
    }
    ws.onmessage = (ev) => {
      const line = String(ev.data)
      setLog((l) => [...l, line])
      const lower = line.toLowerCase()
      if (lower.includes('installation succeeded')) setPhase('success')
      else if (lower.includes('installation failed') || lower.startsWith('error:')) setPhase('error')
      else if (lower.includes('2fa') || lower.includes('verification code')) setPhase('awaiting_2fa')
    }
    ws.onerror = () => setPhase('error')
  }

  const submitOtp = (v: string) => {
    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) return
    wsRef.current.send(JSON.stringify({ t: 2, d: v }))
    setOtp('')
    setPhase('installing')
  }

  const onDrop = useCallback(
    async (files: File[]) => {
      if (files.length === 0) return
      if (!selectedDevice) return toast.error('Pair an Apple TV first')
      if (!account || !password) return toast.error('Enter your Apple ID first')
      const file = files[0]
      if (!file.name.toLowerCase().endsWith('.ipa')) return toast.error('Only .ipa files are supported')

      setLog([`→ Uploading ${file.name} (${(file.size / 1024 / 1024).toFixed(1)} MB)`])
      setPhase('uploading')
      const form = new FormData()
      form.append('files', file)
      try {
        const res = await api<Array<{ path: string; name: string; bundle_identifier?: string; version?: string; icon?: string }>>(
          '/api/upload',
          { method: 'POST', body: form },
        )
        const ipa = Array.isArray(res) && res.length > 0 ? res[0] : null
        if (!ipa?.path) throw new Error('Upload returned no path')
        setLog((l) => [...l, `→ Uploaded ${ipa.name} → ${ipa.path}`])
        startInstall(ipa.path, selectedDevice.udid, {
          name: ipa.name,
          bundle: ipa.bundle_identifier || '',
          version: ipa.version || '',
          icon: ipa.icon || '',
        })
      } catch (e) {
        setPhase('error')
        setLog((l) => [...l, `ERROR: ${e instanceof Error ? e.message : String(e)}`])
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [selectedDevice, account, password],
  )

  const { getRootProps, getInputProps, isDragActive } = useDropzone({
    onDrop,
    accept: { 'application/octet-stream': ['.ipa'] },
    multiple: false,
    disabled: phase === 'uploading' || phase === 'installing' || tvs.length === 0 || !account || !password,
  })

  const active = phase !== 'idle'

  return (
    <div className="space-y-5 h-full flex flex-col">
      <ViewHeader title="Install IPA" subtitle="Sign and sideload an .ipa to a paired Apple TV" />

      {tvs.length === 0 && (
        <EmptyState
          title="No paired device"
          hint="Pair an Apple TV, iPhone or iPad in the Devices tab first."
        />
      )}

      {tvs.length > 0 && (
        <>
          <div className="flex flex-wrap gap-2">
            {tvs.map((d) => {
              const isActive = selected === d.id
              return (
                <button
                  key={d.id}
                  onClick={() => setSelected(d.id)}
                  className={`no-drag inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-[12px] font-medium border transition-colors ${
                    isActive
                      ? 'bg-primary text-primary-foreground border-primary'
                      : 'bg-transparent hover:bg-accent/50 border-border/60'
                  }`}
                >
                  <HugeiconsIcon icon={iconForDevice(d)} size={13} strokeWidth={1.8} />
                  {d.name}
                </button>
              )
            })}
          </div>

          {(!account || !password) && (
            <div className="text-[12px] text-muted-foreground rounded-md border border-dashed border-border/50 px-3 py-2">
              No Apple ID saved — set it in <span className="text-foreground">Settings</span> first.
            </div>
          )}

          <div
            {...getRootProps()}
            className={`no-drag group relative ${active ? 'min-h-[120px]' : 'flex-1 min-h-[220px]'} flex flex-col items-center justify-center rounded-2xl border-2 border-dashed transition-all ${
              !account || !password
                ? 'border-border/30 bg-muted/10 cursor-not-allowed'
                : isDragActive
                ? 'border-primary bg-primary/10 scale-[1.005] cursor-copy'
                : 'border-border/50 hover:border-border hover:bg-accent/20 cursor-pointer'
            } ${active ? 'opacity-70 pointer-events-none' : ''}`}
          >
            <input {...getInputProps()} />
            <HugeiconsIcon
              icon={CloudUploadIcon}
              size={active ? 22 : 28}
              strokeWidth={1.5}
              className="text-muted-foreground mb-2"
            />
            <div className="text-[13px] font-medium">
              {!account || !password
                ? 'Enter your Apple ID above first'
                : isDragActive
                ? 'Release to upload'
                : 'Drag an .ipa here — or click'}
            </div>
            {selectedDevice && !active && (
              <div className="text-[10px] text-muted-foreground/70 mt-2 font-mono">
                → {selectedDevice.name}
              </div>
            )}
          </div>

          {active && (
            <div className="flex-1 min-h-0 flex flex-col rounded-xl border border-border/60 bg-black/20 overflow-hidden">
              <InstallStepper phase={phase} log={log} />
              <div className="flex items-center justify-between px-3 py-2 border-b border-border/40 bg-muted/20">
                <span className="text-[11px] uppercase tracking-wider font-semibold text-muted-foreground">
                  Install log
                </span>
                <span className="text-[11px] font-mono text-muted-foreground/70">
                  {phase}
                </span>
              </div>
              <div className="flex-1 overflow-auto px-3 py-2 font-mono text-[11px] text-muted-foreground/90 space-y-0.5">
                {log.map((l, i) => (
                  <div key={i} className="whitespace-pre-wrap break-all">{l}</div>
                ))}
                <div ref={logEndRef} />
              </div>

              {phase === 'awaiting_2fa' && (
                <div className="border-t border-border/40 px-3 py-3 space-y-2">
                  <div className="text-[12px] text-muted-foreground text-center">
                    Enter the 2FA code sent to your Apple devices
                  </div>
                  <div className="flex justify-center">
                    <InputOTP maxLength={6} value={otp} onChange={(v) => { setOtp(v); if (v.length === 6) submitOtp(v) }} autoFocus>
                      <InputOTPGroup>
                        <InputOTPSlot index={0} />
                        <InputOTPSlot index={1} />
                        <InputOTPSlot index={2} />
                        <InputOTPSlot index={3} />
                        <InputOTPSlot index={4} />
                        <InputOTPSlot index={5} />
                      </InputOTPGroup>
                    </InputOTP>
                  </div>
                </div>
              )}

              {(phase === 'success' || phase === 'error') && (
                <div className="border-t border-border/40 px-3 py-2 flex justify-end">
                  <Button size="sm" variant="outline" onClick={() => { setPhase('idle'); setLog([]) }}>
                    Reset
                  </Button>
                </div>
              )}
            </div>
          )}
        </>
      )}
    </div>
  )
}

function SettingsView() {
  const initial = loadAppleID()
  const [email, setEmail] = useState(initial.email)
  const [password, setPassword] = useState(initial.password)
  const [teamId, setTeamId] = useState(initial.team_id || '')
  const [teams, setTeams] = useState<AppleTeam[]>([])
  const [loadingTeams, setLoadingTeams] = useState(false)
  const [show, setShow] = useState(false)
  const [saved, setSaved] = useState(false)

  const fetchTeams = async () => {
    if (!email) return toast.error('Save Apple ID first')
    setLoadingTeams(true)
    try {
      const res = await api<AppleTeam[]>(`/api/account/teams?email=${encodeURIComponent(email)}`)
      setTeams(res || [])
      if ((res || []).length === 0) toast.error('No teams found (is the account logged in?)')
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to list teams')
    } finally {
      setLoadingTeams(false)
    }
  }
  const [tunneldRunning, setTunneldRunning] = useState(false)
  const [startingTunneld, setStartingTunneld] = useState(false)

  useEffect(() => {
    const check = async () => {
      try {
        const d = await api<{ running: boolean }>('/api/tunneld/status')
        setTunneldRunning(!!d.running)
      } catch {}
    }
    check()
    const t = setInterval(check, 3000)
    return () => clearInterval(t)
  }, [])

  const startTunneld = async () => {
    setStartingTunneld(true)
    try {
      await api('/api/tunneld/start', { method: 'POST' })
      toast.success('Tunneld running')
      setTunneldRunning(true)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to start tunneld')
    } finally {
      setStartingTunneld(false)
    }
  }

  const save = () => {
    saveAppleID({ email: email.trim(), password, team_id: teamId || undefined })
    setSaved(true)
    toast.success('Apple ID saved')
    setTimeout(() => setSaved(false), 1800)
  }

  const clear = () => {
    localStorage.removeItem(SETTINGS_KEY)
    setEmail('')
    setPassword('')
    setTeamId('')
    setTeams([])
    toast.success('Apple ID cleared')
  }

  return (
    <div className="space-y-7">
      <ViewHeader title="Settings" subtitle="Credentials and preferences" />

      <section className="rounded-xl border border-border/60 bg-card/40 backdrop-blur-sm p-5 space-y-4 max-w-2xl">
        <div>
          <h2 className="text-[14px] font-semibold">Apple ID</h2>
          <p className="text-[12px] text-muted-foreground mt-0.5">
            Used to sign IPAs before sideloading. Stored locally on this Mac.
          </p>
        </div>

        <div className="grid grid-cols-2 gap-3">
          <div className="space-y-1.5">
            <label className="block text-[11px] uppercase tracking-wider text-muted-foreground">Email</label>
            <input
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="you@icloud.com"
              className="no-drag w-full rounded-md border border-border/60 bg-background/50 px-3 py-2 text-[13px] outline-none focus:border-primary/60"
            />
          </div>
          <div className="space-y-1.5">
            <label className="block text-[11px] uppercase tracking-wider text-muted-foreground">Password</label>
            <div className="relative">
              <input
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                type={show ? 'text' : 'password'}
                placeholder="app-specific"
                className="no-drag w-full rounded-md border border-border/60 bg-background/50 px-3 py-2 pr-16 text-[13px] outline-none focus:border-primary/60"
              />
              <button
                type="button"
                onClick={() => setShow((s) => !s)}
                className="no-drag absolute right-2 top-1/2 -translate-y-1/2 text-[11px] text-muted-foreground hover:text-foreground"
              >
                {show ? 'Hide' : 'Show'}
              </button>
            </div>
          </div>
        </div>

        <div className="flex items-center gap-2">
          <Button size="sm" onClick={save} className="no-drag" disabled={!email || !password}>
            {saved ? 'Saved ✓' : 'Save'}
          </Button>
          <Button size="sm" variant="ghost" onClick={clear} className="no-drag text-muted-foreground">
            Clear
          </Button>
        </div>

        <div className="space-y-2 pt-3 border-t border-border/40">
          <div className="flex items-center justify-between pt-2">
            <div>
              <label className="block text-[11px] uppercase tracking-wider text-muted-foreground">Developer team</label>
              <p className="text-[11px] text-muted-foreground/70 mt-0.5">Pick which team to sign with.</p>
            </div>
            <Button size="sm" variant="ghost" onClick={fetchTeams} disabled={loadingTeams || !email} className="no-drag text-[11px]">
              {loadingTeams ? 'Loading…' : 'Load teams'}
            </Button>
          </div>
          {teams.length === 0 ? (
            <div className="text-[11px] text-muted-foreground/70">
              Click "Load teams" if your Apple ID has multiple teams and you need to pick one (e.g., personal vs. company).
            </div>
          ) : (
            <div className="grid gap-1">
              {teams.map((t) => {
                const active = teamId === t.id
                return (
                  <button
                    key={t.id}
                    onClick={() => {
                      setTeamId(t.id)
                      saveAppleID({ email: email.trim(), password, team_id: t.id })
                      toast.success(`Selected team ${t.name}`)
                    }}
                    className={`no-drag text-left rounded-md border px-3 py-2 text-[13px] transition-colors ${
                      active
                        ? 'border-primary bg-primary/10'
                        : 'border-border/60 hover:bg-accent/30'
                    }`}
                  >
                    <div className="flex items-center justify-between">
                      <div className="font-medium">{t.name}</div>
                      {active && <div className="text-[10px] text-primary">✓ active</div>}
                    </div>
                    <div className="text-[10px] font-mono text-muted-foreground mt-0.5">{t.id}</div>
                  </button>
                )
              })}
              {teamId && (
                <button
                  onClick={() => setTeamId('')}
                  className="no-drag text-[11px] text-muted-foreground hover:text-foreground text-left pt-1"
                >
                  Clear selection (auto-pick)
                </button>
              )}
            </div>
          )}
        </div>

        <p className="text-[11px] text-muted-foreground/60 pt-1">
          Stored via <code className="font-mono">localStorage</code> inside the app's webview.
        </p>
      </section>

      <section className="rounded-xl border border-border/60 bg-card/40 backdrop-blur-sm p-5 space-y-3 max-w-2xl">
        <div>
          <h2 className="text-[14px] font-semibold">tvOS Install Tunnel</h2>
          <p className="text-[12px] text-muted-foreground mt-0.5">
            Apple TVs on tvOS 17+ require a RemoteXPC tunnel for install pushes. The
            tunnel daemon needs admin rights to create a virtual network interface.
          </p>
        </div>

        <div className="flex items-center gap-3">
          <div className={`h-2 w-2 rounded-full ${tunneldRunning ? 'bg-emerald-500' : 'bg-muted-foreground/40'}`} />
          <span className="text-[12px] text-muted-foreground">
            {tunneldRunning ? 'Tunneld running on 127.0.0.1:49151' : 'Tunneld not running'}
          </span>
          <div className="flex-1" />
          {!tunneldRunning && (
            <Button size="sm" onClick={startTunneld} disabled={startingTunneld} className="no-drag">
              {startingTunneld ? 'Authorizing…' : 'Start (admin)'}
            </Button>
          )}
        </div>
      </section>
    </div>
  )
}

function InstallStepper({ phase, log }: { phase: InstallPhase; log: string[] }) {
  const text = log.join('\n').toLowerCase()
  // Determine the CURRENT step (only one active at a time) by picking the most
  // recent phase marker we saw in the log.
  const markers: { key: string; test: (t: string) => boolean }[] = [
    { key: 'upload', test: (t) => t.includes('→ uploading') },
    { key: 'login', test: (t) => t.includes('phase 0/2') },
    { key: 'register', test: (t) => t.includes('registering device with developer team') },
    { key: 'sign', test: (t) => t.includes('phase 1/2') },
    { key: 'push', test: (t) => t.includes('phase 2/2') },
  ]
  let currentIdx = -1
  markers.forEach((m, i) => { if (m.test(text)) currentIdx = i })
  if (phase === 'uploading') currentIdx = 0
  const done = phase === 'success'
  const failed = phase === 'error'

  const steps = markers.map((_m, i) => {
    let state: 'idle' | 'active' | 'done' | 'error' = 'idle'
    if (done) state = 'done'
    else if (failed && i === currentIdx) state = 'error'
    else if (i < currentIdx) state = 'done'
    else if (i === currentIdx) state = failed ? 'error' : 'active'
    const labels = ['Upload', 'Login', 'Register', 'Sign', 'Push']
    return { label: labels[i], state }
  })

  return (
    <div className="px-4 py-3 border-b border-border/40 bg-background/40">
      <div className="flex items-center gap-1.5">
        {steps.map((s, i) => (
          <div key={s.label} className="flex items-center gap-1.5 flex-1">
            <div
              className={`h-5 w-5 shrink-0 rounded-full flex items-center justify-center text-[9px] font-semibold transition-colors ${
                s.state === 'active'
                  ? 'bg-primary text-primary-foreground animate-pulse'
                  : s.state === 'done'
                  ? 'bg-emerald-500/80 text-white'
                  : s.state === 'error'
                  ? 'bg-destructive text-white'
                  : 'bg-muted text-muted-foreground'
              }`}
            >
              {s.state === 'done' ? '✓' : s.state === 'error' ? '!' : i + 1}
            </div>
            <span className={`text-[11px] ${s.state === 'idle' ? 'text-muted-foreground/60' : 'text-foreground'}`}>
              {s.label}
            </span>
            {i < steps.length - 1 && (
              <div className={`flex-1 h-px ${s.state === 'done' ? 'bg-emerald-500/40' : 'bg-border/40'}`} />
            )}
          </div>
        ))}
      </div>
    </div>
  )
}

type InstalledApp = {
  ID: number
  ipa_name: string
  device: string
  device_class: string
  udid: string
  account: string
  bundle_identifier: string
  version: string
  installed_date?: string
  refreshed_date?: string
  expiration_date?: string
  refreshed_result: boolean
  enabled: boolean
}

function AppsView() {
  const [apps, setApps] = useState<InstalledApp[]>([])
  const [refreshing, setRefreshing] = useState<number | null>(null)

  const load = useCallback(async () => {
    try {
      setApps((await api<InstalledApp[]>('/api/apps')) || [])
    } catch {}
  }, [])

  useEffect(() => {
    load()
    const t = setInterval(load, 10000)
    return () => clearInterval(t)
  }, [load])

  const refresh = async (app: InstalledApp) => {
    setRefreshing(app.ID)
    try {
      await api(`/api/apps/${app.ID}/refresh`, { method: 'POST' })
      toast.success(`Re-signing ${app.ipa_name} queued`)
      load()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Refresh failed')
    } finally {
      setRefreshing(null)
    }
  }

  const reinstall = async (app: InstalledApp) => {
    if (!confirm(`Uninstall ${app.ipa_name} from the device and install again?`)) return
    setRefreshing(app.ID)
    try {
      await api(`/api/apps/${app.ID}/reinstall`, { method: 'POST' })
      toast.success(`Reinstall of ${app.ipa_name} queued`)
      load()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Reinstall failed')
    } finally {
      setRefreshing(null)
    }
  }

  const remove = async (app: InstalledApp) => {
    if (!confirm(`Remove ${app.ipa_name} from refresh queue?`)) return
    // Optimistic: drop the card immediately so duplicates don't look identical
    // while the backend round-trip + 10s poll settle.
    setApps((cur) => cur.filter((a) => a.ID !== app.ID))
    try {
      await api(`/api/apps/${app.ID}/delete`, { method: 'POST' })
      toast.success('Removed')
      load()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Delete failed')
      load()
    }
  }

  const toggleEnabled = async (app: InstalledApp) => {
    try {
      await api(`/api/apps/${app.ID}/toggle`, { method: 'POST' })
      load()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Toggle failed')
    }
  }

  const cleanupDuplicates = async () => {
    // Dedupe key matches the backend: device+class+bundle_id+account
    // (tvOS 17+ RemoteXPC UDIDs rotate, so we can't use udid here).
    const seen = new Set<string>()
    let dupes = 0
    for (const a of apps) {
      const key = `${a.device}|${a.device_class}|${a.bundle_identifier}|${a.account}`
      if (seen.has(key)) dupes++
      else seen.add(key)
    }
    if (dupes === 0) {
      toast.info('No duplicate installs found')
      return
    }
    if (!confirm(`Remove ${dupes} duplicate install${dupes === 1 ? '' : 's'}? The most recent entry per (device + bundle id + account) is kept.`)) return
    try {
      const res = await api<{ deleted: number }>(`/api/apps/cleanup`, { method: 'POST' })
      toast.success(`Removed ${res?.deleted ?? 0} duplicate${res?.deleted === 1 ? '' : 's'}`)
      load()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Cleanup failed')
    }
  }

  return (
    <div className="space-y-5">
      <ViewHeader
        title="Apps"
        subtitle="Installed apps and their 7-day signing lifecycle"
        actions={
          <>
            {apps.length > 1 && (
              <Button variant="outline" size="sm" onClick={cleanupDuplicates} className="no-drag gap-1.5">
                <HugeiconsIcon icon={Delete02Icon} size={14} strokeWidth={1.8} />
                Clean duplicates
              </Button>
            )}
            <Button variant="outline" size="sm" onClick={load} className="no-drag gap-1.5">
              <HugeiconsIcon icon={RefreshIcon} size={14} strokeWidth={1.8} />
              Reload
            </Button>
          </>
        }
      />

      {apps.length === 0 ? (
        <EmptyState title="No apps installed yet" hint="Drop an .ipa in the Install tab to get started." />
      ) : (
        <div className="grid gap-2.5">
          {apps.map((app) => (
            <AppCard
              key={app.ID}
              app={app}
              refreshing={refreshing === app.ID}
              onRefresh={() => refresh(app)}
              onReinstall={() => reinstall(app)}
              onDelete={() => remove(app)}
              onToggle={() => toggleEnabled(app)}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function AppCard({
  app,
  refreshing,
  onRefresh,
  onReinstall,
  onDelete,
  onToggle,
}: {
  app: InstalledApp
  refreshing: boolean
  onRefresh: () => void
  onReinstall: () => void
  onDelete: () => void
  onToggle: () => void
}) {
  const { label, variant } = expiryStatus(app.expiration_date)
  return (
    <div className={`group rounded-lg border border-border/60 bg-card/40 hover:bg-card/60 backdrop-blur-sm transition-colors ${app.enabled === false ? 'opacity-60' : ''}`}>
      <div className="flex items-center gap-3 p-3.5">
        <div className="shrink-0 h-10 w-10 rounded-md bg-muted/60 text-muted-foreground flex items-center justify-center">
          <HugeiconsIcon icon={PackageAddIcon} size={18} strokeWidth={1.6} />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <div className="text-[13px] font-medium truncate">{app.ipa_name}</div>
            {app.version && <span className="text-[10px] text-muted-foreground">v{app.version}</span>}
            {app.enabled === false && (
              <span className="text-[9px] uppercase tracking-wider text-muted-foreground border border-border/60 rounded px-1 py-[1px]">paused</span>
            )}
          </div>
          <div className="mt-0.5 text-[11px] text-muted-foreground truncate">
            {[app.device, app.bundle_identifier, app.account].filter(Boolean).join(' · ')}
          </div>
        </div>
        <div className="shrink-0 flex items-center gap-2">
          <button
            type="button"
            onClick={onToggle}
            title={app.enabled === false ? 'Resume auto re-sign' : 'Pause auto re-sign'}
            className={`no-drag relative inline-flex h-4 w-7 items-center rounded-full transition-colors ${app.enabled === false ? 'bg-muted' : 'bg-emerald-500/80'}`}
          >
            <span
              className={`inline-block h-3 w-3 transform rounded-full bg-white transition-transform ${app.enabled === false ? 'translate-x-0.5' : 'translate-x-3.5'}`}
            />
          </button>
          <span
            className={`inline-flex items-center gap-1 text-[11px] ${
              variant === 'ok'
                ? 'text-emerald-500'
                : variant === 'soon'
                ? 'text-amber-500'
                : variant === 'expired'
                ? 'text-destructive'
                : 'text-muted-foreground'
            }`}
          >
            <span
              className={`h-1.5 w-1.5 rounded-full ${
                variant === 'ok'
                  ? 'bg-emerald-500'
                  : variant === 'soon'
                  ? 'bg-amber-500'
                  : variant === 'expired'
                  ? 'bg-destructive'
                  : 'bg-muted-foreground/40'
              }`}
            />
            {label}
          </span>
          <Button
            size="sm"
            variant="ghost"
            className="no-drag h-7 text-[11px] px-2"
            onClick={onRefresh}
            disabled={refreshing}
            title="Re-sign (extend 7 days)"
          >
            <HugeiconsIcon
              icon={RefreshIcon}
              size={13}
              strokeWidth={1.8}
              className={refreshing ? 'animate-spin' : ''}
            />
          </Button>
          <Button
            size="sm"
            variant="ghost"
            className="no-drag h-7 text-[11px] px-2 opacity-0 group-hover:opacity-100 transition-opacity"
            onClick={onReinstall}
            disabled={refreshing}
            title="Uninstall + install again"
          >
            <HugeiconsIcon icon={PackageReceiveIcon} size={13} strokeWidth={1.8} />
          </Button>
          <Button
            size="sm"
            variant="ghost"
            className="no-drag h-7 text-[11px] px-2 text-muted-foreground hover:text-destructive"
            onClick={onDelete}
            title="Remove from queue"
          >
            <HugeiconsIcon icon={Delete02Icon} size={13} strokeWidth={1.8} />
          </Button>
        </div>
      </div>
    </div>
  )
}

function expiryStatus(iso?: string): { label: string; variant: 'ok' | 'soon' | 'expired' | 'unknown' } {
  if (!iso) return { label: 'unknown', variant: 'unknown' }
  const expires = new Date(iso).getTime()
  const now = Date.now()
  const ms = expires - now
  const days = Math.floor(ms / 86400000)
  if (ms <= 0) return { label: 'expired', variant: 'expired' }
  if (days <= 1) return { label: `expires in ${Math.max(1, Math.round(ms / 3600000))}h`, variant: 'soon' }
  if (days <= 2) return { label: `expires in ${days}d`, variant: 'soon' }
  return { label: `${days}d left`, variant: 'ok' }
}

function LogsView() {
  return (
    <div className="space-y-5">
      <ViewHeader title="Logs" subtitle="Server and install traces" />
      <EmptyState title="No log stream yet" hint="WebSocket tail coming soon." />
    </div>
  )
}

// ViewHeader only renders the actions bar — the page identity is already
// communicated by the active sidebar item, so in-page titles are redundant.
function ViewHeader({ actions }: { title?: string; subtitle?: string; actions?: React.ReactNode }) {
  if (!actions) return null
  return <div className="flex items-center justify-end gap-2">{actions}</div>
}

function EmptyState({ title, hint, compact }: { title: string; hint?: string; compact?: boolean }) {
  return (
    <div className={`rounded-xl border border-dashed border-border/50 text-center ${compact ? 'py-8 px-4' : 'py-16 px-6'}`}>
      <div className="text-[13px] font-medium">{title}</div>
      {hint && <div className="text-[12px] text-muted-foreground mt-1.5">{hint}</div>}
    </div>
  )
}

function mergeDevices(a: Device[], b: Device[]): Device[] {
  // Identity: prefer real UDID, else (name + device_class). The mDNS service
  // fragment / mac_addr is unreliable for tvOS 17+ because the identifier
  // rotates across pairing sessions.
  const keyOf = (d: Device) => {
    const cls = (d.device_class || '').toLowerCase()
    const isUuid = d.udid && /^[0-9a-fA-F-]{20,}$/.test(d.udid)
    if (cls === 'iphone' || cls === 'ipad') {
      return d.udid ? 'u:' + d.udid : 'nc:' + (d.name || '').toLowerCase() + '/' + cls
    }
    // For Apple TVs the rotating identifier isn't stable; key by name+class.
    if (cls === 'appletv') {
      return 'nc:' + (d.name || '').toLowerCase() + '/' + cls
    }
    return isUuid ? 'u:' + d.udid : 'n:' + (d.name || '').toLowerCase()
  }

  const byKey = new Map<string, Device>()
  for (const d of [...a, ...b]) {
    const k = keyOf(d)
    const prev = byKey.get(k)
    if (!prev) {
      byKey.set(k, d)
      continue
    }
    // Merge fields: keep the best of each side. Status=paired wins; richer
    // identifiers (UDID, IP, MAC) from the live scan fill in gaps from the
    // DB-sourced entry.
    const merged: Device = {
      ...prev,
      ...d,
      udid: d.udid || prev.udid,
      ip: (d.ip && d.ip.includes('.')) ? d.ip : (prev.ip || d.ip),
      mac_addr: d.mac_addr || prev.mac_addr,
      status: prev.status === 'paired' ? 'paired' : d.status,
      name: prev.name || d.name,
    }
    byKey.set(k, merged)
  }
  return Array.from(byKey.values())
}

export default function App() {
  const [tab, setTab] = useState<Tab>('devices')
  const [devices, setDevices] = useState<Device[]>([])
  const [loading, setLoading] = useState(true)
  const [pairTarget, setPairTarget] = useState<Device | null>(null)

  const initialLoad = useRef(true)

  const load = useCallback(async () => {
    if (initialLoad.current) setLoading(true)
    try {
      // /api/pair/list is instant (DB read). /api/devices is local usbmuxd.
      // /api/scan/wireless takes ~3s. Fetch all in parallel.
      const [local, wireless, paired] = await Promise.all([
        api<Device[]>('/api/devices').catch(() => null),
        api<Device[]>('/api/scan/wireless?timeout=3').catch(() => null),
        api<Array<{ ID?: number; Name: string; DeviceClass: string; IP: string }>>('/api/pair/list').catch(() => null),
      ])
      const localList = local ?? []
      const wirelessList = wireless ?? []
      // Persisted pairs become authoritative "paired" entries so Install tab is
      // instantly populated even before the mDNS scan catches up.
      const dbPaired: Device[] = (paired ?? []).map((p) => ({
        id: `db-${p.Name}`,
        name: p.Name,
        device_class: p.DeviceClass,
        ip: p.IP,
        udid: '',
        mac_addr: '',
        status: 'paired',
      }))
      if (local === null && wireless === null && (paired === null || paired.length === 0)) return
      setDevices((prev) => {
        const next = mergeDevices(mergeDevices(localList, wirelessList), dbPaired)
        next.sort((a, b) => (a.udid || a.id).localeCompare(b.udid || b.id))
        return next.length === 0 ? prev : next
      })
    } finally {
      initialLoad.current = false
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    load()
    const t = setInterval(load, 12000)
    return () => clearInterval(t)
  }, [load])

  // Re-fetch the moment the user navigates to Install so a just-completed pair
  // surfaces immediately instead of waiting for the next poll.
  useEffect(() => {
    if (tab === 'install') load()
  }, [tab, load])

  const tvCount = devices.filter((d) => (d.device_class || '').toLowerCase() === 'appletv').length

  return (
    <div className="h-screen w-screen flex overflow-hidden">
      <Sidebar tab={tab} setTab={setTab} deviceCount={devices.length} tvCount={tvCount} />

      <main className="flex-1 flex flex-col min-w-0 p-2 pl-0">
        {/* Inset card — rounded like native Mac settings, floats over vibrancy */}
        <div className="flex-1 flex flex-col min-w-0 rounded-xl border border-border/50 bg-card/50 backdrop-blur-md overflow-hidden shadow-[0_0_0_0.5px_rgba(255,255,255,0.04)_inset]">
          {/* Top draggable strip inside the inset */}
          <div className="drag h-10 shrink-0" />
          <section className="flex-1 overflow-auto px-8 pb-8">
            {tab === 'devices' && (
              <DevicesView
                devices={devices}
                loading={loading}
                onRefresh={load}
                onPair={setPairTarget}
                onUnpair={async (d) => {
                  try {
                    await api('/api/pair/delete', {
                      method: 'POST',
                      headers: { 'Content-Type': 'application/json' },
                      body: JSON.stringify({ name: d.name, device_class: d.device_class || 'AppleTV' }),
                    })
                    toast.success(`Unpaired ${d.name}`)
                    load()
                  } catch (e) {
                    toast.error(e instanceof Error ? e.message : 'Unpair failed')
                  }
                }}
              />
            )}
            {tab === 'install' && <InstallView devices={devices} />}
            {tab === 'apps' && <AppsView />}
            {tab === 'logs' && <LogsView />}
            {tab === 'settings' && <SettingsView />}
          </section>
        </div>
      </main>

      <PairDialog device={pairTarget} onClose={() => setPairTarget(null)} onPaired={load} />
      <Toaster position="bottom-right" />
    </div>
  )
}
