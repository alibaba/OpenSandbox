import { useEffect, useRef, useState } from 'react'
import type { ChatMessage } from '@/api/types.ts'

interface Props {
  messages: ChatMessage[]
}

function ToolBlock({ message }: { message: ChatMessage }) {
  const [open, setOpen] = useState(false)
  return (
    <div className="my-1 rounded border border-neutral-700 bg-neutral-900/60">
      <button
        onClick={() => setOpen((o) => !o)}
        className="w-full text-left px-3 py-1.5 text-xs text-neutral-400 flex items-center gap-2 hover:bg-neutral-800/50"
      >
        <span className="text-yellow-500">⚙</span>
        <span className="font-mono">
          {message.toolName ?? (message.role === 'tool_call' ? 'tool_call' : 'tool_result')}
        </span>
        <span className="ml-auto opacity-60">{open ? '▲' : '▼'}</span>
      </button>
      {open && (
        <pre className="px-3 pb-3 text-xs text-neutral-300 font-mono whitespace-pre-wrap break-all overflow-auto max-h-60">
          {message.content}
        </pre>
      )}
    </div>
  )
}

export function MessageList({ messages }: Props) {
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  if (messages.length === 0) {
    return (
      <div className="flex-1 flex items-center justify-center text-neutral-500 text-sm">
        Send a prompt to start a session
      </div>
    )
  }

  return (
    <div className="flex-1 overflow-y-auto px-4 py-3 space-y-3">
      {messages.map((msg) => {
        if (msg.role === 'tool_call' || msg.role === 'tool_result') {
          return <ToolBlock key={msg.id} message={msg} />
        }

        const isUser = msg.role === 'user'
        return (
          <div key={msg.id} className={`flex ${isUser ? 'justify-end' : 'justify-start'}`}>
            <div
              className={`max-w-[80%] rounded-lg px-3 py-2 text-sm ${
                isUser
                  ? 'bg-blue-600/80 text-white'
                  : 'bg-neutral-800 text-neutral-100'
              }`}
            >
              {!isUser && (
                <div className="text-xs text-neutral-500 mb-1 font-mono">assistant</div>
              )}
              <pre className="whitespace-pre-wrap break-words font-sans">
                {msg.content}
                {msg.isStreaming && (
                  <span className="inline-block w-1.5 h-3.5 ml-0.5 bg-neutral-300 animate-pulse align-middle" />
                )}
              </pre>
              <div className="text-xs text-right mt-1 opacity-40">
                {new Date(msg.timestamp).toLocaleTimeString()}
              </div>
            </div>
          </div>
        )
      })}
      <div ref={bottomRef} />
    </div>
  )
}
