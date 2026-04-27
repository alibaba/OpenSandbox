import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { listPools, deletePool, updatePool } from '@/api/pools.ts'
import type { PoolResponse } from '@/api/types.ts'
import { ConfirmDialog } from '@/components/ConfirmDialog.tsx'
import { CreatePoolModal } from '@/components/CreatePoolModal.tsx'
import { SkeletonTable } from '@/components/Skeleton.tsx'
import { PoolDetailDrawer } from '@/features/pools/PoolDetailDrawer.tsx'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Alert, AlertDescription } from '@/components/ui/alert'

export function PoolsPage() {
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
      toast.success('Pool deleted')
    },
    onError: (err: Error) => toast.error(err.message),
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
      toast.success('Pool resized')
    },
    onError: (err: Error) => toast.error(err.message),
  })

  function commitResize(pool: PoolResponse) {
    const raw = editingSize[pool.name]
    if (!raw) return
    const n = parseInt(raw, 10)
    if (isNaN(n) || n < 1) {
      toast.error('Desired size must be ≥ 1')
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
          <Button variant="outline" size="sm" onClick={() => void refetch()}>
            Refresh
          </Button>
          <Button size="sm" onClick={() => setShowCreate(true)}>
            + New Pool
          </Button>
        </div>
      </div>

      {/* Error */}
      {isError && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription className="flex items-center justify-between">
            <span>{(error as Error).message}</span>
            <button onClick={() => void refetch()} className="text-xs underline">
              Retry
            </button>
          </AlertDescription>
        </Alert>
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
                    <div
                      className="flex items-center gap-2"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <Input
                        type="number"
                        min={1}
                        value={editingSize[pool.name] ?? pool.capacitySpec.bufferMax}
                        onChange={(e) =>
                          setEditingSize((prev) => ({ ...prev, [pool.name]: e.target.value }))
                        }
                        className="w-16 h-7 text-xs px-2"
                      />
                      {editingSize[pool.name] !== undefined && (
                        <Button
                          size="sm"
                          className="h-7 text-xs"
                          onClick={() => commitResize(pool)}
                        >
                          Apply
                        </Button>
                      )}
                    </div>
                  </td>
                  <td className="px-4 py-3 text-right" onClick={(e) => e.stopPropagation()}>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="text-xs text-destructive hover:text-destructive h-7"
                      onClick={() => setDeleteTarget(pool.name)}
                    >
                      Delete
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Modals */}
      {showCreate && <CreatePoolModal onClose={() => setShowCreate(false)} />}

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
