import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { createPool } from '@/api/pools.ts'
import type { CreatePoolRequest } from '@/api/types.ts'

interface Props {
  onClose: () => void
  toast: (msg: string, v?: 'success' | 'error' | 'info') => void
}

export function CreatePoolModal({ onClose, toast }: Props) {
  const qc = useQueryClient()
  const [name, setName] = useState('')
  const [bufferMax, setBufferMax] = useState('5')
  const [bufferMin, setBufferMin] = useState('1')
  const [poolMax, setPoolMax] = useState('10')
  const [poolMin, setPoolMin] = useState('0')
  const [templateJson, setTemplateJson] = useState('{\n  "spec": {}\n}')
  const [templateError, setTemplateError] = useState('')

  const mutation = useMutation({
    mutationFn: (req: CreatePoolRequest) => createPool(req),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['pools'] })
      toast('Pool created', 'success')
      onClose()
    },
    onError: (err: Error) => toast(err.message, 'error'),
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setTemplateError('')
    let template: Record<string, unknown>
    try {
      template = JSON.parse(templateJson) as Record<string, unknown>
    } catch {
      setTemplateError('Template must be valid JSON')
      return
    }

    const req: CreatePoolRequest = {
      name: name.trim(),
      template,
      capacitySpec: {
        bufferMax: parseInt(bufferMax, 10),
        bufferMin: parseInt(bufferMin, 10),
        poolMax: parseInt(poolMax, 10),
        poolMin: parseInt(poolMin, 10),
      },
    }
    mutation.mutate(req)
  }

  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-black/60">
      <div className="bg-neutral-900 border border-neutral-700 rounded-lg p-6 w-full max-w-lg shadow-xl">
        <h2 className="text-base font-semibold text-white mb-4">New Pool</h2>
        <form onSubmit={handleSubmit} className="space-y-3">
          <div>
            <label className="block text-xs text-neutral-400 mb-1">Pool Name *</label>
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
              pattern="^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
              placeholder="my-pool"
              className="w-full bg-neutral-800 border border-neutral-600 rounded px-3 py-2 text-sm text-white placeholder-neutral-500 focus:outline-none focus:border-blue-500"
            />
            <p className="text-xs text-neutral-500 mt-1">Lowercase alphanumeric + hyphens</p>
          </div>

          <div className="grid grid-cols-2 gap-3">
            {[
              { label: 'Buffer Max', value: bufferMax, set: setBufferMax },
              { label: 'Buffer Min', value: bufferMin, set: setBufferMin },
              { label: 'Pool Max', value: poolMax, set: setPoolMax },
              { label: 'Pool Min', value: poolMin, set: setPoolMin },
            ].map(({ label, value, set }) => (
              <div key={label}>
                <label className="block text-xs text-neutral-400 mb-1">{label}</label>
                <input
                  type="number"
                  min={0}
                  value={value}
                  onChange={(e) => set(e.target.value)}
                  required
                  className="w-full bg-neutral-800 border border-neutral-600 rounded px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500"
                />
              </div>
            ))}
          </div>

          <div>
            <label className="block text-xs text-neutral-400 mb-1">Pod Template (JSON) *</label>
            <textarea
              value={templateJson}
              onChange={(e) => setTemplateJson(e.target.value)}
              rows={5}
              className="w-full bg-neutral-800 border border-neutral-600 rounded px-3 py-2 text-xs font-mono text-white focus:outline-none focus:border-blue-500 resize-y"
            />
            {templateError && <p className="text-xs text-red-400 mt-1">{templateError}</p>}
          </div>

          <div className="flex gap-3 justify-end pt-2">
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
