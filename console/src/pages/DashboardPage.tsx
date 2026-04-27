import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  listSandboxes,
  deleteSandbox,
  pauseSandbox,
  resumeSandbox,
  renewSandbox,
} from '@/api/sandboxes.ts'
import type { Sandbox } from '@/api/types.ts'
import { StatusBadge } from '@/components/StatusBadge.tsx'
import { ConfirmDialog } from '@/components/ConfirmDialog.tsx'
import { CreateSandboxModal } from '@/components/CreateSandboxModal.tsx'
import { SkeletonTable } from '@/components/Skeleton.tsx'
import { ConsolePanelDrawer } from '@/features/console/ConsolePanelDrawer.tsx'
import { DiagnosticsDrawer } from '@/features/diagnostics/DiagnosticsDrawer.tsx'

interface Props {
  toast: (msg: string, v?: 'success' | 'error' | 'info') => void
}

type ActionMenu = { sandboxId: string; x: number; y: number } | null

export function DashboardPage({ toast }: Props) {
  const qc = useQueryClient()
  const [showCreate, setShowCreate] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)
  const [renewing, setRenewing] = useState<string | null>(null)
  const [actionMenu, setActionMenu] = useState<ActionMenu>(null)
  const [consoleSandbox, setConsoleSandbox] = useState<Sandbox | null>(null)
  const [diagSandbox, setDiagSandbox] = useState<Sandbox | null>(null)

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ['sandboxes'],
    queryFn: () => listSandboxes(),
    refetchInterval: 10_000,
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteSandbox(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['sandboxes'] })
      toast('Sandbox deleted', 'success')
    },
    onError: (err: Error) => toast(err.message, 'error'),
  })

  const pauseMutation = useMutation({
    mutationFn: (id: string) => pauseSandbox(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['sandboxes'] })
      toast('Sandbox paused', 'success')
    },
    onError: (err: Error) => toast(err.message, 'error'),
  })

  const resumeMutation = useMutation({
    mutationFn: (id: string) => resumeSandbox(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['sandboxes'] })
      toast('Sandbox resumed', 'success')
    },
    onError: (err: Error) => toast(err.message, 'error'),
  })

  const renewMutation = useMutation({
    mutationFn: (id: string) => {
      const newExpiry = new Date(Date.now() + 60 * 60 * 1000).toISOString()
      return renewSandbox(id, { expiresAt: newExpiry })
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['sandboxes'] })
      toast('Expiration renewed by 1 hour', 'success')
      setRenewing(null)
    },
    onError: (err: Error) => toast(err.message, 'error'),
  })

  function openMenu(e: React.MouseEvent, sandboxId: string) {
    e.stopPropagation()
    setActionMenu({ sandboxId, x: e.clientX, y: e.clientY })
  }

  function closeMenu() {
    setActionMenu(null)
  }

  function findSandbox(id: string) {
    return data?.items.find((s) => s.id === id) ?? null
  }

  function fmt(ts?: string) {
    if (!ts) return '—'
    return new Date(ts).toLocaleString(undefined, {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    })
  }

  function shortId(id: string) {
    return id.length > 12 ? id.slice(0, 12) + '…' : id
  }

  return (
    <div className="p-6" onClick={closeMenu}>
      {/* Header */}
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-lg font-semibold">Sandboxes</h1>
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
            + New Sandbox
          </button>
        </div>
      </div>

      {/* Error banner */}
      {isError && (
        <div className="mb-4 flex items-center gap-3 px-4 py-3 rounded border border-red-700 bg-red-950/40 text-red-300 text-sm">
          <span className="flex-1">{(error as Error).message}</span>
          <button
            onClick={() => void refetch()}
            className="text-xs underline opacity-80 hover:opacity-100"
          >
            Retry
          </button>
        </div>
      )}

      {/* Table */}
      <div className="rounded-lg border border-neutral-800 overflow-hidden">
        {isLoading ? (
          <SkeletonTable rows={5} />
        ) : !data || data.items.length === 0 ? (
          <div className="py-16 text-center text-neutral-500 text-sm">No sandboxes running</div>
        ) : (
          <table className="w-full text-sm">
            <thead className="bg-neutral-900 text-neutral-400 text-xs">
              <tr>
                <th className="text-left px-4 py-2.5 font-medium">ID</th>
                <th className="text-left px-4 py-2.5 font-medium">Status</th>
                <th className="text-left px-4 py-2.5 font-medium hidden md:table-cell">Image</th>
                <th className="text-left px-4 py-2.5 font-medium hidden lg:table-cell">Created</th>
                <th className="text-left px-4 py-2.5 font-medium hidden lg:table-cell">Expires</th>
                <th className="px-4 py-2.5" />
              </tr>
            </thead>
            <tbody className="divide-y divide-neutral-800">
              {data.items.map((sb) => (
                <tr key={sb.id} className="hover:bg-neutral-900/60 transition-colors">
                  <td className="px-4 py-3 font-mono text-xs text-neutral-300">{shortId(sb.id)}</td>
                  <td className="px-4 py-3">
                    <StatusBadge state={sb.status.state} />
                  </td>
                  <td className="px-4 py-3 text-neutral-400 hidden md:table-cell text-xs truncate max-w-[200px]">
                    {sb.image.uri}
                  </td>
                  <td className="px-4 py-3 text-neutral-400 hidden lg:table-cell text-xs">
                    {fmt(sb.createdAt)}
                  </td>
                  <td className="px-4 py-3 text-neutral-400 hidden lg:table-cell text-xs">
                    {fmt(sb.expiresAt)}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <button
                      onClick={(e) => openMenu(e, sb.id)}
                      className="px-2 py-1 text-xs rounded border border-neutral-700 text-neutral-400 hover:text-white hover:bg-neutral-800"
                    >
                      Actions ▾
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Pagination info */}
      {data && data.items.length > 0 && (
        <p className="mt-2 text-xs text-neutral-500">
          Showing {data.items.length} of {data.pagination.totalItems} sandboxes
        </p>
      )}

      {/* Action menu */}
      {actionMenu && (
        <div
          className="fixed z-30 bg-neutral-900 border border-neutral-700 rounded-lg shadow-xl py-1 w-44"
          style={{ top: actionMenu.y, left: actionMenu.x }}
          onClick={(e) => e.stopPropagation()}
        >
          {[
            {
              label: 'Open Console',
              onClick: () => {
                setConsoleSandbox(findSandbox(actionMenu.sandboxId))
                closeMenu()
              },
            },
            {
              label: 'View Diagnostics',
              onClick: () => {
                setDiagSandbox(findSandbox(actionMenu.sandboxId))
                closeMenu()
              },
            },
            { label: '—', divider: true },
            {
              label: 'Pause',
              onClick: () => {
                pauseMutation.mutate(actionMenu.sandboxId)
                closeMenu()
              },
            },
            {
              label: 'Resume',
              onClick: () => {
                resumeMutation.mutate(actionMenu.sandboxId)
                closeMenu()
              },
            },
            {
              label: 'Renew (+1 hour)',
              onClick: () => {
                setRenewing(actionMenu.sandboxId)
                closeMenu()
              },
            },
            { label: '—', divider: true },
            {
              label: 'Delete',
              danger: true,
              onClick: () => {
                setDeleteTarget(actionMenu.sandboxId)
                closeMenu()
              },
            },
          ].map((item, i) =>
            item.divider ? (
              <hr key={i} className="border-neutral-700 my-1" />
            ) : (
              <button
                key={i}
                onClick={item.onClick}
                className={`w-full text-left px-4 py-1.5 text-xs hover:bg-neutral-800 transition-colors ${
                  item.danger ? 'text-red-400' : 'text-neutral-300'
                }`}
              >
                {item.label}
              </button>
            ),
          )}
        </div>
      )}

      {/* Modals & drawers */}
      {showCreate && (
        <CreateSandboxModal
          onClose={() => setShowCreate(false)}
          onCreated={() => setShowCreate(false)}
          toast={toast}
        />
      )}

      {deleteTarget && (
        <ConfirmDialog
          title="Delete Sandbox"
          message={`Delete sandbox ${shortId(deleteTarget)}? This cannot be undone.`}
          confirmLabel="Delete"
          danger
          onConfirm={() => {
            deleteMutation.mutate(deleteTarget)
            setDeleteTarget(null)
          }}
          onCancel={() => setDeleteTarget(null)}
        />
      )}

      {renewing && (
        <ConfirmDialog
          title="Renew Expiration"
          message={`Extend sandbox ${shortId(renewing)} expiration by 1 hour?`}
          confirmLabel="Renew"
          onConfirm={() => renewMutation.mutate(renewing)}
          onCancel={() => setRenewing(null)}
        />
      )}

      {consoleSandbox && (
        <ConsolePanelDrawer
          sandbox={consoleSandbox}
          onClose={() => setConsoleSandbox(null)}
          toast={toast}
        />
      )}

      {diagSandbox && (
        <DiagnosticsDrawer
          sandbox={diagSandbox}
          onClose={() => setDiagSandbox(null)}
        />
      )}
    </div>
  )
}
