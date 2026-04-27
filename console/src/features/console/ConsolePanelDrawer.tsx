import { useState, useCallback, useRef } from 'react'
import type { Sandbox, ChatMessage } from '@/api/types.ts'
import { streamSession } from '@/api/sse.ts'
import { MessageList } from './MessageList.tsx'
import { PromptInput } from './PromptInput.tsx'
import { ConfirmDialog } from '@/components/ConfirmDialog.tsx'

interface Props {
  sandbox: Sandbox
  onClose: () => void
  toast: (msg: string, v?: 'success' | 'error' | 'info') => void
}

function makeId() {
  return Math.random().toString(36).slice(2)
}

export function ConsolePanelDrawer({ sandbox, onClose, toast }: Props) {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [streaming, setStreaming] = useState(false)
  const [port, setPort] = useState(3000)
  const [confirmClear, setConfirmClear] = useState(false)
  const abortRef = useRef<AbortController | null>(null)

  const isPaused = sandbox.status.state === 'Paused' || sandbox.status.state === 'Pausing'

  const handleSubmit = useCallback(
    (prompt: string) => {
      if (streaming) return

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

      abortRef.current = streamSession(
        sandbox.id,
        port,
        prompt,
        (event) => {
          if (event.type === 'session.message' || event.type === 'message') {
            const data = event.data as Record<string, unknown> | null
            const delta =
              typeof data?.delta === 'string'
                ? data.delta
                : typeof data?.content === 'string'
                  ? data.content
                  : ''
            if (delta) {
              setMessages((prev) =>
                prev.map((m) =>
                  m.id === assistantId
                    ? { ...m, content: m.content + delta }
                    : m,
                ),
              )
            }

            // Tool-use events embedded in the message stream
            if (data?.type === 'tool_call' || data?.type === 'tool_use') {
              const toolMsg: ChatMessage = {
                id: makeId(),
                role: 'tool_call',
                content: JSON.stringify(data?.input ?? data, null, 2),
                timestamp: new Date().toISOString(),
                toolName: data?.name as string | undefined,
              }
              setMessages((prev) => {
                const withoutStreamer = prev.filter((m) => m.id !== assistantId)
                return [...withoutStreamer, toolMsg]
              })
            }

            if (data?.type === 'tool_result') {
              const toolMsg: ChatMessage = {
                id: makeId(),
                role: 'tool_result',
                content: JSON.stringify(data?.content ?? data, null, 2),
                timestamp: new Date().toISOString(),
                toolName: 'result',
              }
              setMessages((prev) => [...prev, toolMsg])
            }
          }
        },
        () => {
          // Done
          setMessages((prev) =>
            prev.map((m) =>
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
          toast(err.message, 'error')
        },
      )
    },
    [sandbox.id, port, streaming, toast],
  )

  function handleClear() {
    abortRef.current?.abort()
    setMessages([])
    setStreaming(false)
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
        <div className="flex items-center gap-3 px-4 py-3 border-b border-neutral-800 shrink-0">
          <div className="flex-1 min-w-0">
            <div className="text-sm font-semibold text-white">Console</div>
            <div className="text-xs text-neutral-500 font-mono truncate">{sandbox.id}</div>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 rounded text-neutral-400 hover:text-white hover:bg-neutral-800"
          >
            ✕
          </button>
        </div>

        {/* Paused warning */}
        {isPaused && (
          <div className="px-4 py-2 bg-yellow-900/40 border-b border-yellow-700/50 text-yellow-300 text-xs">
            Sandbox is paused — resume it before connecting
          </div>
        )}

        {/* Messages */}
        <MessageList messages={messages} />

        {/* Input */}
        <PromptInput
          disabled={streaming || isPaused}
          port={port}
          onPortChange={setPort}
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
