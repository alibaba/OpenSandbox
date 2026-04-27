import { useState, useRef } from 'react'

interface Props {
  disabled: boolean
  port: number
  onPortChange: (port: number) => void
  onSubmit: (prompt: string) => void
  onClear: () => void
  onExport: () => void
}

export function PromptInput({ disabled, port, onPortChange, onSubmit, onClear, onExport }: Props) {
  const [text, setText] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      submit()
    }
  }

  function submit() {
    const trimmed = text.trim()
    if (!trimmed || disabled) return
    onSubmit(trimmed)
    setText('')
    textareaRef.current?.focus()
  }

  return (
    <div className="border-t border-neutral-800 bg-neutral-950">
      {/* Toolbar */}
      <div className="flex items-center gap-3 px-3 pt-2 pb-1">
        <div className="flex items-center gap-1.5 text-xs text-neutral-500">
          <span>Port:</span>
          <input
            type="number"
            value={port}
            onChange={(e) => onPortChange(parseInt(e.target.value, 10) || 3000)}
            className="w-16 bg-neutral-800 border border-neutral-700 rounded px-1.5 py-0.5 text-xs text-white focus:outline-none focus:border-blue-500"
          />
        </div>
        <div className="flex-1" />
        <button
          onClick={onExport}
          className="text-xs text-neutral-500 hover:text-neutral-300 px-2 py-0.5 rounded hover:bg-neutral-800"
        >
          Export
        </button>
        <button
          onClick={onClear}
          className="text-xs text-neutral-500 hover:text-neutral-300 px-2 py-0.5 rounded hover:bg-neutral-800"
        >
          Clear
        </button>
      </div>

      {/* Input area */}
      <div className="flex items-end gap-2 px-3 pb-3">
        <textarea
          ref={textareaRef}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={handleKeyDown}
          disabled={disabled}
          rows={3}
          placeholder={disabled ? 'Waiting for response…' : 'Send a prompt (Enter to send, Shift+Enter for new line)'}
          className="flex-1 bg-neutral-800 border border-neutral-700 rounded px-3 py-2 text-sm text-white placeholder-neutral-500 resize-none focus:outline-none focus:border-blue-500 disabled:opacity-50"
        />
        <button
          onClick={submit}
          disabled={disabled || !text.trim()}
          className="px-3 py-2 rounded bg-blue-600 hover:bg-blue-700 text-white text-sm font-medium disabled:opacity-40 self-end"
        >
          Send
        </button>
      </div>
    </div>
  )
}
