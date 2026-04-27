import { useState } from 'react'
import type { Settings } from '@/api/client.ts'

interface Props {
  initial: Settings
  onSave: (settings: Settings) => void
  onClose: () => void
}

export function SettingsPanel({ initial, onSave, onClose }: Props) {
  const [serverUrl, setServerUrl] = useState(initial.serverUrl)
  const [authToken, setAuthToken] = useState(initial.authToken)

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!serverUrl.trim()) return
    onSave({ serverUrl: serverUrl.trim(), authToken: authToken.trim() })
    onClose()
  }

  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-black/60">
      <div className="bg-neutral-900 border border-neutral-700 rounded-lg p-6 w-full max-w-md shadow-xl">
        <h2 className="text-lg font-semibold text-white mb-1">Connection Settings</h2>
        <p className="text-xs text-neutral-500 mb-5">
          Settings are stored locally in your browser and never sent to a third party.
        </p>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-sm text-neutral-300 mb-1">
              OpenSandbox Server URL <span className="text-red-400">*</span>
            </label>
            <input
              type="url"
              value={serverUrl}
              onChange={(e) => setServerUrl(e.target.value)}
              placeholder="http://localhost:8090"
              required
              className="w-full bg-neutral-800 border border-neutral-600 rounded px-3 py-2 text-sm text-white placeholder-neutral-500 focus:outline-none focus:border-blue-500"
            />
          </div>

          <div>
            <label className="block text-sm text-neutral-300 mb-1">
              Bearer Token <span className="text-neutral-500">(optional)</span>
            </label>
            <input
              type="password"
              value={authToken}
              onChange={(e) => setAuthToken(e.target.value)}
              placeholder="sk-..."
              className="w-full bg-neutral-800 border border-neutral-600 rounded px-3 py-2 text-sm text-white placeholder-neutral-500 focus:outline-none focus:border-blue-500"
            />
          </div>

          <div className="flex gap-3 justify-end pt-2">
            {initial.serverUrl && (
              <button
                type="button"
                onClick={onClose}
                className="px-4 py-1.5 text-sm rounded border border-neutral-600 text-neutral-300 hover:bg-neutral-800"
              >
                Cancel
              </button>
            )}
            <button
              type="submit"
              className="px-4 py-1.5 text-sm rounded bg-blue-600 hover:bg-blue-700 text-white font-medium"
            >
              Save
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
