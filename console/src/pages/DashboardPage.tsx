import {
  deleteSandbox,
  listSandboxes,
  pauseSandbox,
  renewSandbox,
  resumeSandbox,
} from '@/api/sandboxes.ts'
import type { Sandbox } from '@/api/types.ts'
import { ConfirmDialog } from '@/components/ConfirmDialog.tsx'
import { CreateSandboxModal } from '@/components/CreateSandboxModal.tsx'
import { SkeletonTable } from '@/components/Skeleton.tsx'
import { StatusBadge } from '@/components/StatusBadge.tsx'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { ConsolePanelDrawer } from '@/features/console/ConsolePanelDrawer.tsx'
import { DiagnosticsDrawer } from '@/features/diagnostics/DiagnosticsDrawer.tsx'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { toast } from 'sonner'

export function DashboardPage() {
  const qc = useQueryClient()
  const [showCreate, setShowCreate] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)
  const [renewing, setRenewing] = useState<string | null>(null)
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
      toast.success('Sandbox deleted')
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const pauseMutation = useMutation({
    mutationFn: (id: string) => pauseSandbox(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['sandboxes'] })
      toast.success('Sandbox paused')
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const resumeMutation = useMutation({
    mutationFn: (id: string) => resumeSandbox(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['sandboxes'] })
      toast.success('Sandbox resumed')
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const renewMutation = useMutation({
    mutationFn: (id: string) => {
      const newExpiry = new Date(Date.now() + 60 * 60 * 1000).toISOString()
      return renewSandbox(id, { expiresAt: newExpiry })
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['sandboxes'] })
      toast.success('Expiration renewed by 1 hour')
      setRenewing(null)
    },
    onError: (err: Error) => toast.error(err.message),
  })

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
    <div className="p-6">
      {/* Header */}
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-lg font-semibold">Sandboxes</h1>
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => void refetch()}
          >
            Refresh
          </Button>
          <Button size="sm" onClick={() => setShowCreate(true)}>
            + New Sandbox
          </Button>
        </div>
      </div>

      {/* Error banner */}
      {isError && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription className="flex items-center justify-between">
            <span>{(error as Error).message}</span>
            <button
              onClick={() => void refetch()}
              className="text-xs underline opacity-80 hover:opacity-100"
            >
              Retry
            </button>
          </AlertDescription>
        </Alert>
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
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="outline" size="sm" className="text-xs h-7">
                          Actions
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem onClick={() => setConsoleSandbox(sb)}>
                          Open Console
                        </DropdownMenuItem>
                        <DropdownMenuItem onClick={() => setDiagSandbox(sb)}>
                          View Diagnostics
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem onClick={() => pauseMutation.mutate(sb.id)}>
                          Pause
                        </DropdownMenuItem>
                        <DropdownMenuItem onClick={() => resumeMutation.mutate(sb.id)}>
                          Resume
                        </DropdownMenuItem>
                        <DropdownMenuItem onClick={() => setRenewing(sb.id)}>
                          Renew (+1 hour)
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                          className="text-destructive focus:text-destructive"
                          onClick={() => setDeleteTarget(sb.id)}
                        >
                          Delete
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
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

      {/* Modals & drawers */}
      {showCreate && (
        <CreateSandboxModal
          onClose={() => setShowCreate(false)}
          onCreated={() => setShowCreate(false)}
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
