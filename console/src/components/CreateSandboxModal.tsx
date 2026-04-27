import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { createSandbox } from '@/api/sandboxes.ts'
import type {
  CreateSandboxRequest,
  NetworkRule,
  Volume,
  VolumeHost,
  VolumePVC,
  VolumeOSSFS,
} from '@/api/types.ts'

interface Props {
  onClose: () => void
  onCreated: () => void
}

type Tab = 'basic' | 'resources' | 'environment' | 'network' | 'volumes' | 'advanced'

const TABS: { id: Tab; label: string }[] = [
  { id: 'basic', label: 'Basic' },
  { id: 'resources', label: 'Resources' },
  { id: 'environment', label: 'Environment' },
  { id: 'network', label: 'Network' },
  { id: 'volumes', label: 'Volumes' },
  { id: 'advanced', label: 'Advanced' },
]

type KVRow = { key: string; value: string }

type VolumeBackend = 'host' | 'pvc' | 'ossfs'

interface VolumeState {
  name: string
  mountPath: string
  readOnly: boolean
  subPath: string
  backend: VolumeBackend
  host: { path: string }
  pvc: {
    claimName: string
    createIfNotExists: boolean
    deleteOnSandboxTermination: boolean
    storageClass: string
    storage: string
    accessModes: string
  }
  ossfs: {
    bucket: string
    endpoint: string
    version: '1.0' | '2.0'
    options: string
    accessKeyId: string
    accessKeySecret: string
  }
}

function emptyVolume(): VolumeState {
  return {
    name: '',
    mountPath: '',
    readOnly: false,
    subPath: '',
    backend: 'pvc',
    host: { path: '' },
    pvc: {
      claimName: '',
      createIfNotExists: true,
      deleteOnSandboxTermination: false,
      storageClass: '',
      storage: '',
      accessModes: '',
    },
    ossfs: {
      bucket: '',
      endpoint: '',
      version: '2.0',
      options: '',
      accessKeyId: '',
      accessKeySecret: '',
    },
  }
}

const inputCls =
  'w-full bg-neutral-800 border border-neutral-600 rounded px-3 py-2 text-sm text-white placeholder-neutral-500 focus:outline-none focus:border-blue-500'
const labelCls = 'block text-xs text-neutral-400 mb-1'
const selectCls =
  'bg-neutral-800 border border-neutral-600 rounded px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500'

export function CreateSandboxModal({ onClose, onCreated }: Props) {
  const qc = useQueryClient()
  const [tab, setTab] = useState<Tab>('basic')

  // Basic
  const [imageUri, setImageUri] = useState('opensandbox/code-interpreter:local')
  const [entrypoint, setEntrypoint] = useState('/entrypoint.sh')
  const [useAuth, setUseAuth] = useState(false)
  const [authUser, setAuthUser] = useState('')
  const [authPass, setAuthPass] = useState('')
  const [platformOs, setPlatformOs] = useState('')
  const [platformArch, setPlatformArch] = useState('')

  // Resources
  const [cpu, setCpu] = useState('')
  const [memory, setMemory] = useState('')
  const [timeout, setTimeout_] = useState('')

  // Environment
  const [envRows, setEnvRows] = useState<KVRow[]>([{ key: '', value: '' }])

  // Network
  const [secureAccess, setSecureAccess] = useState(false)
  const [netDefaultAction, setNetDefaultAction] = useState<'allow' | 'deny' | ''>('')
  const [egressRules, setEgressRules] = useState<NetworkRule[]>([])

  // Volumes
  const [volumes, setVolumes] = useState<VolumeState[]>([])

  // Advanced
  const [metaRows, setMetaRows] = useState<KVRow[]>([{ key: '', value: '' }])
  const [extRows, setExtRows] = useState<KVRow[]>([{ key: '', value: '' }])

  const mutation = useMutation({
    mutationFn: (req: CreateSandboxRequest) => createSandbox(req),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['sandboxes'] })
      toast.success('Sandbox created')
      onCreated()
    },
    onError: (err: Error) => toast.error(err.message),
  })

  function kvToRecord(rows: KVRow[]): Record<string, string> | null {
    const entries = rows.filter((r) => r.key.trim())
    return entries.length > 0
      ? Object.fromEntries(entries.map((r) => [r.key.trim(), r.value]))
      : null
  }

  function buildVolume(vs: VolumeState): Volume {
    const v: Volume = {
      name: vs.name.trim(),
      mountPath: vs.mountPath.trim(),
    }
    if (vs.readOnly) v.readOnly = true
    if (vs.subPath.trim()) v.subPath = vs.subPath.trim()

    if (vs.backend === 'host') {
      const hb: VolumeHost = { path: vs.host.path.trim() }
      v.host = hb
    } else if (vs.backend === 'pvc') {
      const pb: VolumePVC = { claimName: vs.pvc.claimName.trim() }
      pb.createIfNotExists = vs.pvc.createIfNotExists
      pb.deleteOnSandboxTermination = vs.pvc.deleteOnSandboxTermination
      if (vs.pvc.storageClass.trim()) pb.storageClass = vs.pvc.storageClass.trim()
      if (vs.pvc.storage.trim()) pb.storage = vs.pvc.storage.trim()
      if (vs.pvc.accessModes.trim()) {
        pb.accessModes = vs.pvc.accessModes
          .split(',')
          .map((m) => m.trim())
          .filter(Boolean)
      }
      v.pvc = pb
    } else {
      const ob: VolumeOSSFS = {
        bucket: vs.ossfs.bucket.trim(),
        endpoint: vs.ossfs.endpoint.trim(),
        version: vs.ossfs.version,
      }
      if (vs.ossfs.options.trim()) {
        ob.options = vs.ossfs.options
          .split(',')
          .map((o) => o.trim())
          .filter(Boolean)
      }
      if (vs.ossfs.accessKeyId.trim()) ob.accessKeyId = vs.ossfs.accessKeyId.trim()
      if (vs.ossfs.accessKeySecret.trim()) ob.accessKeySecret = vs.ossfs.accessKeySecret.trim()
      v.ossfs = ob
    }
    return v
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!imageUri.trim() || !entrypoint.trim()) {
      setTab('basic')
      return
    }

    const resourceLimits: Record<string, string> = {}
    if (cpu.trim()) resourceLimits['cpu'] = cpu.trim()
    if (memory.trim()) resourceLimits['memory'] = memory.trim()

    const req: CreateSandboxRequest = {
      image: {
        uri: imageUri.trim(),
        ...(useAuth && authUser.trim()
          ? { auth: { username: authUser.trim(), password: authPass } }
          : {}),
      },
      resourceLimits,
      entrypoint: entrypoint.trim().split(/\s+/),
    }

    if (platformOs.trim() || platformArch.trim()) {
      req.platform = { os: platformOs.trim(), arch: platformArch.trim() }
    }

    if (timeout) req.timeout = parseInt(timeout, 10)

    const env = kvToRecord(envRows)
    if (env) req.env = env

    const metadata = kvToRecord(metaRows)
    if (metadata) req.metadata = metadata

    const extensions = kvToRecord(extRows)
    if (extensions) req.extensions = extensions

    if (netDefaultAction || egressRules.length > 0) {
      req.networkPolicy = {
        ...(netDefaultAction ? { defaultAction: netDefaultAction } : {}),
        ...(egressRules.length > 0 ? { egress: egressRules } : {}),
      }
    }

    if (secureAccess) req.secureAccess = true

    const builtVolumes = volumes
      .filter((v) => v.name.trim() && v.mountPath.trim())
      .map(buildVolume)
    if (builtVolumes.length > 0) req.volumes = builtVolumes

    mutation.mutate(req)
  }

  // KV editor helpers
  function updateKV(
    rows: KVRow[],
    setRows: (r: KVRow[]) => void,
    i: number,
    field: 'key' | 'value',
    val: string,
  ) {
    setRows(rows.map((r, idx) => (idx === i ? { ...r, [field]: val } : r)))
  }

  function removeKV(rows: KVRow[], setRows: (r: KVRow[]) => void, i: number) {
    setRows(rows.filter((_, idx) => idx !== i))
  }

  function addKV(rows: KVRow[], setRows: (r: KVRow[]) => void) {
    setRows([...rows, { key: '', value: '' }])
  }

  // Volume helpers
  function updateVolume<K extends keyof VolumeState>(i: number, field: K, val: VolumeState[K]) {
    setVolumes(volumes.map((v, idx) => (idx === i ? { ...v, [field]: val } : v)))
  }

  function updateVolumeNested<
    Section extends 'host' | 'pvc' | 'ossfs',
    K extends keyof VolumeState[Section],
  >(i: number, section: Section, field: K, val: VolumeState[Section][K]) {
    setVolumes(
      volumes.map((v, idx) =>
        idx === i ? { ...v, [section]: { ...v[section], [field]: val } } : v,
      ),
    )
  }

  return (
    <div
      className="fixed inset-0 z-40 flex items-center justify-center bg-black/60"
      onClick={onClose}
    >
      <div
        className="bg-neutral-900 border border-neutral-700 rounded-lg shadow-xl w-full max-w-2xl flex flex-col"
        style={{ maxHeight: '90vh' }}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-6 pt-5 pb-3 border-b border-neutral-800">
          <h2 className="text-base font-semibold text-white">New Sandbox</h2>
          <button
            onClick={onClose}
            className="text-neutral-500 hover:text-white text-lg leading-none"
          >
            ×
          </button>
        </div>

        {/* Tabs */}
        <div className="flex border-b border-neutral-800 px-6 gap-1 shrink-0">
          {TABS.map((t) => (
            <button
              key={t.id}
              type="button"
              onClick={() => setTab(t.id)}
              className={`px-3 py-2 text-xs font-medium border-b-2 -mb-px transition-colors ${
                tab === t.id
                  ? 'border-blue-500 text-white'
                  : 'border-transparent text-neutral-400 hover:text-neutral-200'
              }`}
            >
              {t.label}
            </button>
          ))}
        </div>

        {/* Body */}
        <form onSubmit={handleSubmit} className="flex flex-col flex-1 overflow-hidden">
          <div className="flex-1 overflow-y-auto px-6 py-4 space-y-3">
            {/* ── Basic ── */}
            {tab === 'basic' && (
              <>
                <div>
                  <label className={labelCls}>Image URI *</label>
                  <input
                    value={imageUri}
                    onChange={(e) => setImageUri(e.target.value)}
                    required
                    className={inputCls}
                  />
                </div>

                <div>
                  <label className={labelCls}>Entrypoint *</label>
                  <input
                    value={entrypoint}
                    onChange={(e) => setEntrypoint(e.target.value)}
                    required
                    placeholder="/entrypoint.sh"
                    className={inputCls}
                  />
                  <p className="text-xs text-neutral-500 mt-1">Space-separated command + args</p>
                </div>

                <div>
                  <label className="flex items-center gap-2 text-xs text-neutral-400 cursor-pointer select-none">
                    <input
                      type="checkbox"
                      checked={useAuth}
                      onChange={(e) => setUseAuth(e.target.checked)}
                      className="accent-blue-500"
                    />
                    Private registry — provide credentials
                  </label>
                </div>

                {useAuth && (
                  <div className="pl-4 space-y-3 border-l-2 border-neutral-700">
                    <div>
                      <label className={labelCls}>Registry username</label>
                      <input
                        value={authUser}
                        onChange={(e) => setAuthUser(e.target.value)}
                        className={inputCls}
                      />
                    </div>
                    <div>
                      <label className={labelCls}>Registry password / token</label>
                      <input
                        type="password"
                        value={authPass}
                        onChange={(e) => setAuthPass(e.target.value)}
                        className={inputCls}
                      />
                    </div>
                  </div>
                )}

                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className={labelCls}>Platform OS</label>
                    <input
                      value={platformOs}
                      onChange={(e) => setPlatformOs(e.target.value)}
                      placeholder="linux"
                      className={inputCls}
                    />
                  </div>
                  <div>
                    <label className={labelCls}>Platform Arch</label>
                    <input
                      value={platformArch}
                      onChange={(e) => setPlatformArch(e.target.value)}
                      placeholder="amd64"
                      className={inputCls}
                    />
                  </div>
                </div>
              </>
            )}

            {/* ── Resources ── */}
            {tab === 'resources' && (
              <>
                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className={labelCls}>CPU limit</label>
                    <input
                      value={cpu}
                      onChange={(e) => setCpu(e.target.value)}
                      placeholder="e.g. 500m"
                      className={inputCls}
                    />
                  </div>
                  <div>
                    <label className={labelCls}>Memory limit</label>
                    <input
                      value={memory}
                      onChange={(e) => setMemory(e.target.value)}
                      placeholder="e.g. 512Mi"
                      className={inputCls}
                    />
                  </div>
                </div>
                <div>
                  <label className={labelCls}>Timeout (seconds)</label>
                  <input
                    type="number"
                    value={timeout}
                    onChange={(e) => setTimeout_(e.target.value)}
                    placeholder="No timeout"
                    min={60}
                    className={inputCls}
                  />
                  <p className="text-xs text-neutral-500 mt-1">
                    Minimum 60 s. Leave blank for no auto-termination.
                  </p>
                </div>
              </>
            )}

            {/* ── Environment ── */}
            {tab === 'environment' && (
              <>
                <p className="text-xs text-neutral-500">
                  Environment variables injected into the sandbox.
                </p>
                <div className="space-y-2">
                  {envRows.map((row, i) => (
                    <div key={i} className="flex gap-2 items-center">
                      <input
                        value={row.key}
                        onChange={(e) => updateKV(envRows, setEnvRows, i, 'key', e.target.value)}
                        placeholder="KEY"
                        className={`${inputCls} font-mono`}
                      />
                      <input
                        value={row.value}
                        onChange={(e) =>
                          updateKV(envRows, setEnvRows, i, 'value', e.target.value)
                        }
                        placeholder="value"
                        className={inputCls}
                      />
                      <button
                        type="button"
                        onClick={() => removeKV(envRows, setEnvRows, i)}
                        className="text-neutral-500 hover:text-red-400 shrink-0 px-1"
                      >
                        ×
                      </button>
                    </div>
                  ))}
                </div>
                <button
                  type="button"
                  onClick={() => addKV(envRows, setEnvRows)}
                  className="text-xs text-blue-400 hover:text-blue-300"
                >
                  + Add variable
                </button>
              </>
            )}

            {/* ── Network ── */}
            {tab === 'network' && (
              <>
                <div>
                  <label className="flex items-center gap-2 text-xs text-neutral-400 cursor-pointer select-none">
                    <input
                      type="checkbox"
                      checked={secureAccess}
                      onChange={(e) => setSecureAccess(e.target.checked)}
                      className="accent-blue-500"
                    />
                    Secure Access (K8s ingress gateway mode)
                  </label>
                </div>

                <div className="pt-2 border-t border-neutral-800">
                  <p className="text-xs text-neutral-400 font-medium mb-3">Network Policy</p>

                  <div className="mb-3">
                    <label className={labelCls}>Default outbound action</label>
                    <select
                      value={netDefaultAction}
                      onChange={(e) =>
                        setNetDefaultAction(e.target.value as 'allow' | 'deny' | '')
                      }
                      className={`${selectCls} w-full`}
                    >
                      <option value="">— unset (allow all) —</option>
                      <option value="allow">allow</option>
                      <option value="deny">deny</option>
                    </select>
                  </div>

                  <p className="text-xs text-neutral-500 mb-2">Egress rules (evaluated in order)</p>
                  <div className="space-y-2">
                    {egressRules.map((rule, i) => (
                      <div key={i} className="flex gap-2 items-center">
                        <select
                          value={rule.action}
                          onChange={(e) =>
                            setEgressRules(
                              egressRules.map((r, idx) =>
                                idx === i
                                  ? { ...r, action: e.target.value as 'allow' | 'deny' }
                                  : r,
                              ),
                            )
                          }
                          className={`${selectCls} w-24 shrink-0`}
                        >
                          <option value="allow">allow</option>
                          <option value="deny">deny</option>
                        </select>
                        <input
                          value={rule.target}
                          onChange={(e) =>
                            setEgressRules(
                              egressRules.map((r, idx) =>
                                idx === i ? { ...r, target: e.target.value } : r,
                              ),
                            )
                          }
                          placeholder="example.com or *.example.com"
                          className={inputCls}
                        />
                        <button
                          type="button"
                          onClick={() =>
                            setEgressRules(egressRules.filter((_, idx) => idx !== i))
                          }
                          className="text-neutral-500 hover:text-red-400 shrink-0 px-1"
                        >
                          ×
                        </button>
                      </div>
                    ))}
                  </div>
                  <button
                    type="button"
                    onClick={() =>
                      setEgressRules([...egressRules, { action: 'allow', target: '' }])
                    }
                    className="text-xs text-blue-400 hover:text-blue-300 mt-2"
                  >
                    + Add rule
                  </button>
                </div>
              </>
            )}

            {/* ── Volumes ── */}
            {tab === 'volumes' && (
              <>
                {volumes.length === 0 && (
                  <p className="text-xs text-neutral-500">
                    No volumes configured. Click below to add one.
                  </p>
                )}
                {volumes.map((vol, i) => (
                  <div key={i} className="border border-neutral-700 rounded-lg p-4 space-y-3">
                    <div className="flex items-center justify-between mb-1">
                      <span className="text-xs font-medium text-neutral-300">Volume {i + 1}</span>
                      <button
                        type="button"
                        onClick={() => setVolumes(volumes.filter((_, idx) => idx !== i))}
                        className="text-xs text-neutral-500 hover:text-red-400"
                      >
                        Remove
                      </button>
                    </div>

                    <div className="grid grid-cols-2 gap-3">
                      <div>
                        <label className={labelCls}>Name *</label>
                        <input
                          value={vol.name}
                          onChange={(e) => updateVolume(i, 'name', e.target.value)}
                          placeholder="my-volume"
                          className={inputCls}
                        />
                        <p className="text-xs text-neutral-500 mt-1">
                          Lowercase alphanumeric + hyphens
                        </p>
                      </div>
                      <div>
                        <label className={labelCls}>Mount path *</label>
                        <input
                          value={vol.mountPath}
                          onChange={(e) => updateVolume(i, 'mountPath', e.target.value)}
                          placeholder="/data"
                          className={inputCls}
                        />
                      </div>
                    </div>

                    <div className="grid grid-cols-2 gap-3">
                      <div>
                        <label className={labelCls}>Sub path</label>
                        <input
                          value={vol.subPath}
                          onChange={(e) => updateVolume(i, 'subPath', e.target.value)}
                          placeholder="optional subdirectory"
                          className={inputCls}
                        />
                      </div>
                      <div className="flex items-center gap-2 pt-5">
                        <input
                          type="checkbox"
                          id={`readonly-${i}`}
                          checked={vol.readOnly}
                          onChange={(e) => updateVolume(i, 'readOnly', e.target.checked)}
                          className="accent-blue-500"
                        />
                        <label
                          htmlFor={`readonly-${i}`}
                          className="text-xs text-neutral-400 cursor-pointer"
                        >
                          Read only
                        </label>
                      </div>
                    </div>

                    <div>
                      <label className={labelCls}>Backend</label>
                      <select
                        value={vol.backend}
                        onChange={(e) =>
                          updateVolume(i, 'backend', e.target.value as VolumeBackend)
                        }
                        className={`${selectCls} w-full`}
                      >
                        <option value="host">host (bind mount)</option>
                        <option value="pvc">pvc (named volume / PVC)</option>
                        <option value="ossfs">ossfs (Alibaba OSS)</option>
                      </select>
                    </div>

                    {vol.backend === 'host' && (
                      <div>
                        <label className={labelCls}>Host path *</label>
                        <input
                          value={vol.host.path}
                          onChange={(e) => updateVolumeNested(i, 'host', 'path', e.target.value)}
                          placeholder="/host/path"
                          className={inputCls}
                        />
                      </div>
                    )}

                    {vol.backend === 'pvc' && (
                      <div className="space-y-3">
                        <div>
                          <label className={labelCls}>Claim name *</label>
                          <input
                            value={vol.pvc.claimName}
                            onChange={(e) =>
                              updateVolumeNested(i, 'pvc', 'claimName', e.target.value)
                            }
                            placeholder="my-pvc"
                            className={inputCls}
                          />
                        </div>
                        <div className="grid grid-cols-2 gap-3">
                          <div>
                            <label className={labelCls}>Storage class</label>
                            <input
                              value={vol.pvc.storageClass}
                              onChange={(e) =>
                                updateVolumeNested(i, 'pvc', 'storageClass', e.target.value)
                              }
                              placeholder="default"
                              className={inputCls}
                            />
                          </div>
                          <div>
                            <label className={labelCls}>Storage size</label>
                            <input
                              value={vol.pvc.storage}
                              onChange={(e) =>
                                updateVolumeNested(i, 'pvc', 'storage', e.target.value)
                              }
                              placeholder="1Gi"
                              className={inputCls}
                            />
                          </div>
                        </div>
                        <div>
                          <label className={labelCls}>Access modes (comma-separated)</label>
                          <input
                            value={vol.pvc.accessModes}
                            onChange={(e) =>
                              updateVolumeNested(i, 'pvc', 'accessModes', e.target.value)
                            }
                            placeholder="ReadWriteOnce"
                            className={inputCls}
                          />
                        </div>
                        <div className="flex gap-4">
                          <label className="flex items-center gap-2 text-xs text-neutral-400 cursor-pointer">
                            <input
                              type="checkbox"
                              checked={vol.pvc.createIfNotExists}
                              onChange={(e) =>
                                updateVolumeNested(
                                  i,
                                  'pvc',
                                  'createIfNotExists',
                                  e.target.checked,
                                )
                              }
                              className="accent-blue-500"
                            />
                            Auto-create if absent
                          </label>
                          <label className="flex items-center gap-2 text-xs text-neutral-400 cursor-pointer">
                            <input
                              type="checkbox"
                              checked={vol.pvc.deleteOnSandboxTermination}
                              onChange={(e) =>
                                updateVolumeNested(
                                  i,
                                  'pvc',
                                  'deleteOnSandboxTermination',
                                  e.target.checked,
                                )
                              }
                              className="accent-blue-500"
                            />
                            Delete on termination
                          </label>
                        </div>
                      </div>
                    )}

                    {vol.backend === 'ossfs' && (
                      <div className="space-y-3">
                        <div className="grid grid-cols-2 gap-3">
                          <div>
                            <label className={labelCls}>Bucket *</label>
                            <input
                              value={vol.ossfs.bucket}
                              onChange={(e) =>
                                updateVolumeNested(i, 'ossfs', 'bucket', e.target.value)
                              }
                              placeholder="my-bucket"
                              className={inputCls}
                            />
                          </div>
                          <div>
                            <label className={labelCls}>Endpoint *</label>
                            <input
                              value={vol.ossfs.endpoint}
                              onChange={(e) =>
                                updateVolumeNested(i, 'ossfs', 'endpoint', e.target.value)
                              }
                              placeholder="oss-cn-hangzhou.aliyuncs.com"
                              className={inputCls}
                            />
                          </div>
                        </div>
                        <div className="grid grid-cols-2 gap-3">
                          <div>
                            <label className={labelCls}>Access key ID</label>
                            <input
                              value={vol.ossfs.accessKeyId}
                              onChange={(e) =>
                                updateVolumeNested(i, 'ossfs', 'accessKeyId', e.target.value)
                              }
                              className={inputCls}
                            />
                          </div>
                          <div>
                            <label className={labelCls}>Access key secret</label>
                            <input
                              type="password"
                              value={vol.ossfs.accessKeySecret}
                              onChange={(e) =>
                                updateVolumeNested(
                                  i,
                                  'ossfs',
                                  'accessKeySecret',
                                  e.target.value,
                                )
                              }
                              className={inputCls}
                            />
                          </div>
                        </div>
                        <div className="grid grid-cols-2 gap-3">
                          <div>
                            <label className={labelCls}>Version</label>
                            <select
                              value={vol.ossfs.version}
                              onChange={(e) =>
                                updateVolumeNested(
                                  i,
                                  'ossfs',
                                  'version',
                                  e.target.value as '1.0' | '2.0',
                                )
                              }
                              className={`${selectCls} w-full`}
                            >
                              <option value="2.0">2.0</option>
                              <option value="1.0">1.0</option>
                            </select>
                          </div>
                          <div>
                            <label className={labelCls}>Options (comma-separated)</label>
                            <input
                              value={vol.ossfs.options}
                              onChange={(e) =>
                                updateVolumeNested(i, 'ossfs', 'options', e.target.value)
                              }
                              placeholder="allow_other,umask=022"
                              className={inputCls}
                            />
                          </div>
                        </div>
                      </div>
                    )}
                  </div>
                ))}

                <button
                  type="button"
                  onClick={() => setVolumes([...volumes, emptyVolume()])}
                  className="text-xs text-blue-400 hover:text-blue-300"
                >
                  + Add volume
                </button>
              </>
            )}

            {/* ── Advanced ── */}
            {tab === 'advanced' && (
              <>
                <div>
                  <p className="text-xs text-neutral-400 font-medium mb-2">Metadata</p>
                  <p className="text-xs text-neutral-500 mb-2">
                    Custom key-value tags for management and filtering.
                  </p>
                  <div className="space-y-2">
                    {metaRows.map((row, i) => (
                      <div key={i} className="flex gap-2 items-center">
                        <input
                          value={row.key}
                          onChange={(e) =>
                            updateKV(metaRows, setMetaRows, i, 'key', e.target.value)
                          }
                          placeholder="key"
                          className={inputCls}
                        />
                        <input
                          value={row.value}
                          onChange={(e) =>
                            updateKV(metaRows, setMetaRows, i, 'value', e.target.value)
                          }
                          placeholder="value"
                          className={inputCls}
                        />
                        <button
                          type="button"
                          onClick={() => removeKV(metaRows, setMetaRows, i)}
                          className="text-neutral-500 hover:text-red-400 shrink-0 px-1"
                        >
                          ×
                        </button>
                      </div>
                    ))}
                  </div>
                  <button
                    type="button"
                    onClick={() => addKV(metaRows, setMetaRows)}
                    className="text-xs text-blue-400 hover:text-blue-300 mt-2"
                  >
                    + Add entry
                  </button>
                </div>

                <div className="pt-4 border-t border-neutral-800">
                  <p className="text-xs text-neutral-400 font-medium mb-2">Extensions</p>
                  <p className="text-xs text-neutral-500 mb-2">
                    Provider-specific or transient parameters not in the core API.
                  </p>
                  <div className="space-y-2">
                    {extRows.map((row, i) => (
                      <div key={i} className="flex gap-2 items-center">
                        <input
                          value={row.key}
                          onChange={(e) =>
                            updateKV(extRows, setExtRows, i, 'key', e.target.value)
                          }
                          placeholder="key"
                          className={inputCls}
                        />
                        <input
                          value={row.value}
                          onChange={(e) =>
                            updateKV(extRows, setExtRows, i, 'value', e.target.value)
                          }
                          placeholder="value"
                          className={inputCls}
                        />
                        <button
                          type="button"
                          onClick={() => removeKV(extRows, setExtRows, i)}
                          className="text-neutral-500 hover:text-red-400 shrink-0 px-1"
                        >
                          ×
                        </button>
                      </div>
                    ))}
                  </div>
                  <button
                    type="button"
                    onClick={() => addKV(extRows, setExtRows)}
                    className="text-xs text-blue-400 hover:text-blue-300 mt-2"
                  >
                    + Add entry
                  </button>
                </div>
              </>
            )}
          </div>

          {/* Footer */}
          <div className="flex gap-3 justify-end px-6 py-4 border-t border-neutral-800 shrink-0">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-1.5 text-sm rounded border border-neutral-600 text-neutral-300 hover:bg-neutral-800"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={mutation.isPending}
              className="px-4 py-1.5 text-sm rounded bg-blue-600 hover:bg-blue-700 text-white font-medium disabled:opacity-50"
            >
              {mutation.isPending ? 'Creating…' : 'Create'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
