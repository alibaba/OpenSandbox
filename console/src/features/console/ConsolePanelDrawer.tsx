import { useState, useCallback, useRef } from 'react'
import { toast } from 'sonner'
import type { Sandbox, ChatMessage } from '@/api/types.ts'
import { streamSession } from '@/api/sse.ts'
import { MessageList } from './MessageList.tsx'
import { PromptInput } from './PromptInput.tsx'
import { ConfirmDialog } from '@/components/ConfirmDialog.tsx'

interface Props {
  sandbox: Sandbox
  onClose: () => void
}

function makeId() {
  return Math.random().toString(36).slice(2)
}

function defaultCwd(sandbox: Sandbox): string {
  const { USERNAME, SESSION_ID } = sandbox.env ?? {}
  if (USERNAME && SESSION_ID) return `/workspace/${USERNAME}/${SESSION_ID}`
  if (USERNAME) return `/workspace/${USERNAME}`
  return '/workspace'
}

export function ConsolePanelDrawer({ sandbox, onClose }: Props) {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [streaming, setStreaming] = useState(false)
  const [port, setPort] = useState(3000)
  const [confirmClear, setConfirmClear] = useState(false)
  const [sessionId, setSessionId] = useState<string | null>(null)
  const [cwd, setCwd] = useState(() => defaultCwd(sandbox))
  const [permissionMode, setPermissionMode] = useState('acceptEdits')
  const [permissionDeniedMsg, setPermissionDeniedMsg] = useState<string | null>(null)
  const [showEnv, setShowEnv] = useState(false)
  const abortRef = useRef<AbortController | null>(null)
  const firstAssistantRef = useRef(false)

  const isPaused = sandbox.status.state === 'Paused' || sandbox.status.state === 'Pausing'
  const envEntries = Object.entries(sandbox.env ?? {})

  const handleSubmit = useCallback(
    (prompt: string, permMode?: string) => {
      if (streaming) return
      setPermissionDeniedMsg(null)

      const userMsg: ChatMessage = {
        id: makeId(),
        role: 'user',
        content: prompt,
        timestamp: new Date().toISOString(),
      }
      const assistantId = makeId()
      const assistantMsg: ChatMessage = {
        id: assistantId,
        role: 'assistant',
        content: '',
        timestamp: new Date().toISOString(),
        isStreaming: true,
      }
      setMessages((prev) => [...prev, userMsg, assistantMsg])
      setStreaming(true)
      firstAssistantRef.current = false

      abortRef.current = streamSession(
        sandbox.id,
        port,
        sessionId,
        prompt,
        cwd,
        permMode ?? permissionMode,
        (event) => {
          const data = event.data as Record<string, unknown> | null

          // Detect permission-denied errors from tool results
          if (event.type === 'message.raw') {
            const rawMsg = data?.message as Record<string, unknown> | null
            if (rawMsg?.type === 'user') {
              const userMsg = rawMsg.message as { content?: unknown[] } | null
              const contents = Array.isArray(userMsg?.content) ? userMsg!.content : []
              for (const block of contents) {
                const b = block as Record<string, unknown>
                if (b.type === 'tool_result' && b.is_error === true) {
                  const text = typeof b.content === 'string' ? b.content : ''
                  if (text.includes('requested permissions')) {
                    setPermissionDeniedMsg(text)
                  }
                }
              }
            }
            return
          }

          // Streaming text delta (only when includePartialMessages is true)
          if (event.type === 'message.delta') {
            const streamEvent = data?.event as Record<string, unknown> | null
            if (streamEvent?.type === 'content_block_delta') {
              const delta = streamEvent.delta as Record<string, unknown> | null
              if (delta?.type === 'text_delta' && typeof delta.text === 'string') {
                setMessages((prev) =>
                  prev.map((m) =>
                    m.id === assistantId
                      ? { ...m, content: m.content + (delta.text as string) }
                      : m,
                  ),
                )
              }
            }
            return
          }

          // Complete assistant message
          if (event.type === 'message.assistant') {
            const text = typeof data?.text === 'string' ? data.text : ''

            if (!firstAssistantRef.current) {
              // First assistant message: update the pre-created placeholder
              firstAssistantRef.current = true
              setMessages((prev) =>
                prev.map((m) =>
                  m.id === assistantId ? { ...m, content: text } : m,
                ),
              )
            } else if (text) {
              // Subsequent assistant messages (multi-turn tool use): new bubble
              setMessages((prev) => [
                ...prev,
                {
                  id: makeId(),
                  role: 'assistant' as const,
                  content: text,
                  timestamp: new Date().toISOString(),
                },
              ])
            }

            // Extract tool_use blocks from message.content
            const messageObj = data?.message as { content?: unknown[] } | null
            const contentBlocks = Array.isArray(messageObj?.content) ? messageObj!.content : []
            const toolUseBlocks = contentBlocks.filter(
              (b): b is { type: 'tool_use'; id: string; name: string; input: unknown } =>
                typeof b === 'object' && b !== null && (b as Record<string, unknown>).type === 'tool_use',
            )
            if (toolUseBlocks.length > 0) {
              setMessages((prev) => [
                ...prev,
                ...toolUseBlocks.map((block) => ({
                  id: makeId(),
                  role: 'tool_call' as const,
                  content: JSON.stringify(block.input, null, 2),
                  timestamp: new Date().toISOString(),
                  toolName: block.name,
                })),
              ])
            }
            return
          }
        },
        (id) => {
          setSessionId(id)
        },
        () => {
          // Done — remove the placeholder if it was never filled (tool-only turn)
          setMessages((prev) =>
            prev
              .filter((m) => m.id !== assistantId || m.content !== '')
              .map((m) =>
                m.id === assistantId ? { ...m, isStreaming: false } : m,
              ),
          )
          setStreaming(false)
        },
        (err) => {
          setMessages((prev) =>
            prev.map((m) =>
              m.id === assistantId
                ? { ...m, content: `[Error] ${err.message}`, isStreaming: false }
                : m,
            ),
          )
          setStreaming(false)
          toast.error(err.message)
        },
      )
    },
    [sandbox.id, port, sessionId, cwd, permissionMode, streaming],
  )

  function handleClear() {
    abortRef.current?.abort()
    setMessages([])
    setStreaming(false)
    setSessionId(null)
    setPermissionDeniedMsg(null)
    firstAssistantRef.current = false
    setConfirmClear(false)
  }

  function handleExport() {
    const lines = messages.map((m) => {
      const role = m.role.toUpperCase()
      const ts = new Date(m.timestamp).toLocaleTimeString()
      return `## ${role} [${ts}]\n\n${m.content}\n`
    })
    const blob = new Blob([lines.join('\n---\n\n')], { type: 'text/markdown' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `console-${sandbox.id.slice(0, 8)}-${Date.now()}.md`
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <div className="fixed inset-0 z-40 flex">
      {/* Backdrop */}
      <div className="flex-1 bg-black/40" onClick={onClose} />

      {/* Panel */}
      <div className="w-full max-w-2xl bg-neutral-950 border-l border-neutral-800 flex flex-col shadow-2xl">
        {/* Header */}
        <div className="px-4 py-3 border-b border-neutral-800 shrink-0">
          <div className="flex items-center gap-3">
            <div className="flex-1 min-w-0">
              <div className="text-sm font-semibold text-white">Console</div>
              <div className="text-xs text-neutral-500 font-mono truncate">{sandbox.id}</div>
            </div>
            {envEntries.length > 0 && (
              <button
                onClick={() => setShowEnv((v) => !v)}
                className="text-xs text-neutral-500 hover:text-neutral-300 px-2 py-0.5 rounded hover:bg-neutral-800 font-mono"
              >
                ENV {showEnv ? '▲' : '▼'}
              </button>
            )}
            <button
              onClick={onClose}
              className="p-1.5 rounded text-neutral-400 hover:text-white hover:bg-neutral-800"
            >
              ✕
            </button>
          </div>

          {/* Env vars panel */}
          {showEnv && envEntries.length > 0 && (
            <div className="mt-2 rounded bg-neutral-900 border border-neutral-800 px-3 py-2 max-h-40 overflow-y-auto">
              {envEntries.map(([key, val]) => (
                <div key={key} className="flex gap-2 text-xs font-mono leading-5 min-w-0">
                  <span className="text-blue-400 shrink-0">{key}</span>
                  <span className="text-neutral-500">=</span>
                  <span className="text-neutral-300 truncate">{val}</span>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Paused warning */}
        {isPaused && (
          <div className="px-4 py-2 bg-yellow-900/40 border-b border-yellow-700/50 text-yellow-300 text-xs">
            Sandbox is paused — resume it before connecting
          </div>
        )}

        {/* Permission denied banner */}
        {permissionDeniedMsg && !streaming && (
          <div className="px-4 py-2 bg-orange-900/40 border-b border-orange-700/50 text-xs">
            <div className="text-orange-300 mb-1.5">⚠ {permissionDeniedMsg}</div>
            <div className="flex gap-2">
              <button
                onClick={() => {
                  setPermissionMode('acceptEdits')
                  setPermissionDeniedMsg(null)
                  handleSubmit('Please retry the last action.', 'acceptEdits')
                }}
                className="px-2 py-0.5 rounded bg-orange-700 hover:bg-orange-600 text-white text-xs font-medium"
              >
                Allow &amp; Retry
              </button>
              <button
                onClick={() => setPermissionDeniedMsg(null)}
                className="px-2 py-0.5 rounded hover:bg-neutral-800 text-neutral-400 text-xs"
              >
                Dismiss
              </button>
            </div>
          </div>
        )}

        {/* Messages */}
        <MessageList messages={messages} />

        {/* Input */}
        <PromptInput
          disabled={streaming || isPaused}
          port={port}
          onPortChange={setPort}
          cwd={cwd}
          onCwdChange={setCwd}
          permissionMode={permissionMode}
          onPermissionModeChange={setPermissionMode}
          sessionStarted={sessionId !== null}
          onSubmit={handleSubmit}
          onClear={() => setConfirmClear(true)}
          onExport={handleExport}
        />
      </div>

      {confirmClear && (
        <ConfirmDialog
          title="Clear Transcript"
          message="Clear all messages? Session state on the server is not affected."
          confirmLabel="Clear"
          onConfirm={handleClear}
          onCancel={() => setConfirmClear(false)}
        />
      )}
    </div>
  )
}
