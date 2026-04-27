import type { PoolResponse } from '@/api/types.ts'

interface Props {
  pool: PoolResponse
  onClose: () => void
}

export function PoolDetailDrawer({ pool, onClose }: Props) {
  return (
    <div className="fixed inset-0 z-40 flex">
      <div className="flex-1 bg-black/40" onClick={onClose} />
      <div className="w-full max-w-xl bg-neutral-950 border-l border-neutral-800 flex flex-col shadow-2xl overflow-y-auto">
        <div className="flex items-center gap-3 px-4 py-3 border-b border-neutral-800 shrink-0">
          <div className="flex-1">
            <div className="text-sm font-semibold text-white">Pool: {pool.name}</div>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 rounded text-neutral-400 hover:text-white hover:bg-neutral-800"
          >
            ✕
          </button>
        </div>

        <div className="p-4 space-y-4">
          {/* Capacity */}
          <section>
            <h3 className="text-xs font-semibold text-neutral-400 uppercase tracking-wider mb-2">
              Capacity
            </h3>
            <div className="grid grid-cols-2 gap-2">
              {Object.entries(pool.capacitySpec).map(([k, v]) => (
                <div key={k} className="bg-neutral-900 rounded px-3 py-2">
                  <div className="text-xs text-neutral-500">{k}</div>
                  <div className="text-sm font-mono text-white">{v}</div>
                </div>
              ))}
            </div>
          </section>

          {/* Status */}
          {pool.status && (
            <section>
              <h3 className="text-xs font-semibold text-neutral-400 uppercase tracking-wider mb-2">
                Runtime Status
              </h3>
              <div className="grid grid-cols-2 gap-2">
                {Object.entries(pool.status).map(([k, v]) => (
                  <div key={k} className="bg-neutral-900 rounded px-3 py-2">
                    <div className="text-xs text-neutral-500">{k}</div>
                    <div className="text-sm font-mono text-white">{String(v)}</div>
                  </div>
                ))}
              </div>
            </section>
          )}

          {/* Full config */}
          <section>
            <h3 className="text-xs font-semibold text-neutral-400 uppercase tracking-wider mb-2">
              Full Config (JSON)
            </h3>
            <pre className="bg-neutral-900 rounded p-3 text-xs font-mono text-neutral-300 overflow-auto max-h-80">
              {JSON.stringify(pool, null, 2)}
            </pre>
          </section>
        </div>
      </div>
    </div>
  )
}
