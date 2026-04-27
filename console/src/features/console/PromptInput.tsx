import { useState, useRef } from 'react'

const PERMISSION_MODES = [
  { value: 'acceptEdits', label: 'Accept Edits' },
  { value: 'auto', label: 'Auto' },
  { value: 'default', label: 'Prompt' },
  { value: 'plan', label: 'Plan' },
] as const

interface Props {
  disabled: boolean
  port: number
  onPortChange: (port: number) => void
  cwd: string
  onCwdChange: (cwd: string) => void
  permissionMode: string
  onPermissionModeChange: (mode: string) => void
  sessionStarted: boolean
  onSubmit: (prompt: string) => void
  onClear: () => void
  onExport: () => void
}

export function PromptInput({
  disabled,
  port,
  onPortChange,
  cwd,
  onCwdChange,
  permissionMode,
  onPermissionModeChange,
  sessionStarted,
  onSubmit,
  onClear,
  onExport,
}: Props) {
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
      <div className="flex items-center gap-3 px-3 pt-2 pb-1 flex-wrap">
        <div className="flex items-center gap-1.5 text-xs text-neutral-500">
          <span>Port:</span>
          <input
            type="number"
            value={port}
            onChange={(e) => onPortChange(parseInt(e.target.value, 10) || 3000)}
            className="w-16 bg-neutral-800 border border-neutral-700 rounded px-1.5 py-0.5 text-xs text-white focus:outline-none focus:border-blue-500"
          />
        </div>
        <div className="flex items-center gap-1.5 text-xs text-neutral-500 flex-1 min-w-0">
          <span className="shrink-0">CWD:</span>
          <input
            type="text"
            value={cwd}
            onChange={(e) => onCwdChange(e.target.value)}
            disabled={sessionStarted}
            title={sessionStarted ? 'Working directory is fixed for the current session. Clear transcript to change.' : 'Working directory for new session'}
            className="flex-1 min-w-0 bg-neutral-800 border border-neutral-700 rounded px-1.5 py-0.5 text-xs text-white font-mono focus:outline-none focus:border-blue-500 disabled:opacity-40 disabled:cursor-not-allowed"
          />
        </div>
        <div className="flex items-center gap-1.5 text-xs text-neutral-500">
          <span className="shrink-0">Perms:</span>
          <select
            value={permissionMode}
            onChange={(e) => onPermissionModeChange(e.target.value)}
            className="bg-neutral-800 border border-neutral-700 rounded px-1.5 py-0.5 text-xs text-white focus:outline-none focus:border-blue-500"
          >
            {PERMISSION_MODES.map((m) => (
              <option key={m.value} value={m.value}>{m.label}</option>
            ))}
          </select>
        </div>
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
