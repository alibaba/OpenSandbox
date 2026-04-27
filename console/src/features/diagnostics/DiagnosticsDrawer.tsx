import { useState, useEffect, useRef } from 'react'
import { FixedSizeList as List } from 'react-window'
import type { Sandbox } from '@/api/types.ts'
import {
  getSandboxLogs,
  getSandboxInspect,
  getSandboxEvents,
  getSandboxDiagnosticsSummary,
} from '@/api/devops.ts'
import { ApiRequestError } from '@/api/client.ts'

const MAX_LINES = 5000

type Tab = 'logs' | 'inspect' | 'events' | 'summary'

interface Props {
  sandbox: Sandbox
  onClose: () => void
}

function TabButton({ label, active, onClick }: { label: string; active: boolean; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className={`px-4 py-2 text-xs font-medium border-b-2 transition-colors ${
        active
          ? 'border-blue-500 text-blue-400'
          : 'border-transparent text-neutral-500 hover:text-neutral-300'
      }`}
    >
      {label}
    </button>
  )
}

function LogsTab({ sandboxId }: { sandboxId: string }) {
  const [lines, setLines] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [truncated, setTruncated] = useState(false)
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  async function fetchLogs() {
    try {
      const text = await getSandboxLogs(sandboxId, 200)
      const newLines = text.split('\n').filter(Boolean)
      setLines((prev) => {
        const combined = [...prev, ...newLines.filter((l) => !prev.includes(l))]
        if (combined.length > MAX_LINES) {
          setTruncated(true)
          return combined.slice(-MAX_LINES)
        }
        return combined
      })
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load logs')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void fetchLogs()
    intervalRef.current = setInterval(() => void fetchLogs(), 3000)
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sandboxId])

  if (loading) return <div className="p-4 text-neutral-500 text-xs">Loading logs…</div>
  if (error) return <div className="p-4 text-red-400 text-xs">{error}</div>
  if (lines.length === 0)
    return <div className="p-4 text-neutral-500 text-xs">No log output yet</div>

  return (
    <div className="flex flex-col h-full">
      {truncated && (
        <div className="px-4 py-1.5 bg-yellow-900/30 border-b border-yellow-700/30 text-yellow-400 text-xs">
          Older lines truncated (showing last {MAX_LINES})
        </div>
      )}
      <List
        height={500}
        itemCount={lines.length}
        itemSize={18}
        width="100%"
        className="font-mono"
      >
        {({ index, style }) => (
          <div style={style} className="px-4 text-xs text-neutral-300 whitespace-nowrap overflow-hidden text-ellipsis">
            {lines[index]}
          </div>
        )}
      </List>
    </div>
  )
}

function InspectTab({ sandboxId }: { sandboxId: string }) {
  const [text, setText] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    getSandboxInspect(sandboxId)
      .then(setText)
      .catch((err: unknown) => setError(err instanceof Error ? err.message : 'Failed'))
  }, [sandboxId])

  function copy() {
    if (!text) return
    void navigator.clipboard.writeText(text).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  if (!text && !error) return <div className="p-4 text-neutral-500 text-xs">Loading…</div>
  if (error) return <div className="p-4 text-red-400 text-xs">{error}</div>

  return (
    <div className="relative flex flex-col h-full">
      <div className="absolute top-2 right-4">
        <button
          onClick={copy}
          className="text-xs px-2 py-1 rounded bg-neutral-800 text-neutral-400 hover:text-white border border-neutral-700"
        >
          {copied ? 'Copied!' : 'Copy'}
        </button>
      </div>
      <pre className="flex-1 p-4 text-xs font-mono text-neutral-300 overflow-auto whitespace-pre-wrap break-all">
        {text}
      </pre>
    </div>
  )
}

function EventsTab({ sandboxId }: { sandboxId: string }) {
  const [text, setText] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    getSandboxEvents(sandboxId)
      .then(setText)
      .catch((err: unknown) => setError(err instanceof Error ? err.message : 'Failed'))
  }, [sandboxId])

  if (!text && !error) return <div className="p-4 text-neutral-500 text-xs">Loading…</div>
  if (error) return <div className="p-4 text-red-400 text-xs">{error}</div>
  if (!text?.trim()) return <div className="p-4 text-neutral-500 text-xs">No events recorded</div>

  const events = text.trim().split('\n').filter(Boolean)
  return (
    <div className="overflow-y-auto p-4 space-y-1">
      {events.map((line, i) => (
        <div key={i} className="text-xs font-mono text-neutral-300 px-3 py-1.5 rounded bg-neutral-900/60 border border-neutral-800">
          {line}
        </div>
      ))}
    </div>
  )
}

function SummaryTab({ sandboxId }: { sandboxId: string }) {
  const [text, setText] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    getSandboxDiagnosticsSummary(sandboxId)
      .then(setText)
      .catch((err: unknown) => {
        const msg =
          err instanceof ApiRequestError
            ? `Error ${err.status}: ${err.message}`
            : err instanceof Error
              ? err.message
              : 'Failed'
        setError(msg)
      })
  }, [sandboxId])

  if (!text && !error) return <div className="p-4 text-neutral-500 text-xs">Loading…</div>
  if (error) return <div className="p-4 text-red-400 text-xs">{error}</div>

  return (
    <pre className="p-4 text-xs font-mono text-neutral-300 overflow-auto whitespace-pre-wrap">
      {text}
    </pre>
  )
}

export function DiagnosticsDrawer({ sandbox, onClose }: Props) {
  const [tab, setTab] = useState<Tab>('logs')

  return (
    <div className="fixed inset-0 z-40 flex">
      <div className="flex-1 bg-black/40" onClick={onClose} />
      <div className="w-full max-w-2xl bg-neutral-950 border-l border-neutral-800 flex flex-col shadow-2xl">
        {/* Header */}
        <div className="flex items-center gap-3 px-4 py-3 border-b border-neutral-800 shrink-0">
          <div className="flex-1 min-w-0">
            <div className="text-sm font-semibold text-white">Diagnostics</div>
            <div className="text-xs text-neutral-500 font-mono truncate">{sandbox.id}</div>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 rounded text-neutral-400 hover:text-white hover:bg-neutral-800"
          >
            ✕
          </button>
        </div>

        {/* Tabs */}
        <div className="flex border-b border-neutral-800 shrink-0">
          {(['logs', 'inspect', 'events', 'summary'] as Tab[]).map((t) => (
            <TabButton
              key={t}
              label={t.charAt(0).toUpperCase() + t.slice(1)}
              active={tab === t}
              onClick={() => setTab(t)}
            />
          ))}
        </div>

        {/* Content */}
        <div className="flex-1 overflow-auto">
          {tab === 'logs' && <LogsTab sandboxId={sandbox.id} />}
          {tab === 'inspect' && <InspectTab sandboxId={sandbox.id} />}
          {tab === 'events' && <EventsTab sandboxId={sandbox.id} />}
          {tab === 'summary' && <SummaryTab sandboxId={sandbox.id} />}
        </div>
      </div>
    </div>
  )
}
