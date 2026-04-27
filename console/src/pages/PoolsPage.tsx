import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { listPools, deletePool, updatePool } from '@/api/pools.ts'
import type { PoolResponse } from '@/api/types.ts'
import { ConfirmDialog } from '@/components/ConfirmDialog.tsx'
import { CreatePoolModal } from '@/components/CreatePoolModal.tsx'
import { SkeletonTable } from '@/components/Skeleton.tsx'
import { PoolDetailDrawer } from '@/features/pools/PoolDetailDrawer.tsx'

interface Props {
  toast: (msg: string, v?: 'success' | 'error' | 'info') => void
}

export function PoolsPage({ toast }: Props) {
  const qc = useQueryClient()
  const [showCreate, setShowCreate] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)
  const [detailPool, setDetailPool] = useState<PoolResponse | null>(null)
  const [editingSize, setEditingSize] = useState<Record<string, string>>({})

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ['pools'],
    queryFn: listPools,
  })

  const deleteMutation = useMutation({
    mutationFn: (name: string) => deletePool(name),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['pools'] })
      toast('Pool deleted', 'success')
    },
    onError: (err: Error) => toast(err.message, 'error'),
  })

  const resizeMutation = useMutation({
    mutationFn: ({ name, bufferMax }: { name: string; bufferMax: number }) =>
      updatePool(name, {
        capacitySpec: {
          bufferMax,
          bufferMin: 1,
          poolMax: bufferMax * 2,
          poolMin: 0,
        },
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['pools'] })
      toast('Pool resized', 'success')
    },
    onError: (err: Error) => toast(err.message, 'error'),
  })

  function commitResize(pool: PoolResponse) {
    const raw = editingSize[pool.name]
    if (!raw) return
    const n = parseInt(raw, 10)
    if (isNaN(n) || n < 1) {
      toast('Desired size must be ≥ 1', 'error')
      return
    }
    resizeMutation.mutate({ name: pool.name, bufferMax: n })
    setEditingSize((prev) => {
      const next = { ...prev }
      delete next[pool.name]
      return next
    })
  }

  return (
    <div className="p-6">
      {/* Header */}
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-lg font-semibold">Pools</h1>
        <div className="flex gap-2">
          <button
            onClick={() => void refetch()}
            className="px-3 py-1.5 text-xs rounded border border-neutral-700 text-neutral-400 hover:text-white hover:bg-neutral-800"
          >
            Refresh
          </button>
          <button
            onClick={() => setShowCreate(true)}
            className="px-3 py-1.5 text-xs rounded bg-blue-600 hover:bg-blue-700 text-white font-medium"
          >
            + New Pool
          </button>
        </div>
      </div>

      {/* Error */}
      {isError && (
        <div className="mb-4 flex items-center gap-3 px-4 py-3 rounded border border-red-700 bg-red-950/40 text-red-300 text-sm">
          <span className="flex-1">{(error as Error).message}</span>
          <button onClick={() => void refetch()} className="text-xs underline">
            Retry
          </button>
        </div>
      )}

      {/* Table */}
      <div className="rounded-lg border border-neutral-800 overflow-hidden">
        {isLoading ? (
          <SkeletonTable rows={3} />
        ) : !data || data.items.length === 0 ? (
          <div className="py-16 text-center text-neutral-500 text-sm">No pools configured</div>
        ) : (
          <table className="w-full text-sm">
            <thead className="bg-neutral-900 text-neutral-400 text-xs">
              <tr>
                <th className="text-left px-4 py-2.5 font-medium">Name</th>
                <th className="text-left px-4 py-2.5 font-medium">Total</th>
                <th className="text-left px-4 py-2.5 font-medium">Available</th>
                <th className="text-left px-4 py-2.5 font-medium">Buffer Max</th>
                <th className="px-4 py-2.5" />
              </tr>
            </thead>
            <tbody className="divide-y divide-neutral-800">
              {data.items.map((pool) => (
                <tr
                  key={pool.name}
                  className="hover:bg-neutral-900/60 transition-colors cursor-pointer"
                  onClick={() => setDetailPool(pool)}
                >
                  <td className="px-4 py-3 font-mono text-sm text-neutral-300">{pool.name}</td>
                  <td className="px-4 py-3 text-neutral-400 text-sm">
                    {pool.status?.total ?? '—'}
                  </td>
                  <td className="px-4 py-3 text-neutral-400 text-sm">
                    {pool.status?.available ?? '—'}
                  </td>
                  <td className="px-4 py-3">
                    {/* Inline resize */}
                    <div
                      className="flex items-center gap-2"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <input
                        type="number"
                        min={1}
                        value={editingSize[pool.name] ?? pool.capacitySpec.bufferMax}
                        onChange={(e) =>
                          setEditingSize((prev) => ({ ...prev, [pool.name]: e.target.value }))
                        }
                        className="w-16 bg-neutral-800 border border-neutral-700 rounded px-2 py-1 text-xs text-white focus:outline-none focus:border-blue-500"
                      />
                      {editingSize[pool.name] !== undefined && (
                        <button
                          onClick={() => commitResize(pool)}
                          className="text-xs px-2 py-1 bg-blue-600 hover:bg-blue-700 text-white rounded"
                        >
                          Apply
                        </button>
                      )}
                    </div>
                  </td>
                  <td className="px-4 py-3 text-right" onClick={(e) => e.stopPropagation()}>
                    <button
                      onClick={() => setDeleteTarget(pool.name)}
                      className="text-xs text-red-400 hover:text-red-300 px-2 py-1 rounded hover:bg-neutral-800"
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Modals */}
      {showCreate && <CreatePoolModal onClose={() => setShowCreate(false)} toast={toast} />}

      {deleteTarget && (
        <ConfirmDialog
          title="Delete Pool"
          message={`Delete pool "${deleteTarget}"?`}
          confirmLabel="Delete"
          danger
          onConfirm={() => {
            deleteMutation.mutate(deleteTarget)
            setDeleteTarget(null)
          }}
          onCancel={() => setDeleteTarget(null)}
        />
      )}

      {detailPool && (
        <PoolDetailDrawer pool={detailPool} onClose={() => setDetailPool(null)} />
      )}
    </div>
  )
}
