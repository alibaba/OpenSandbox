import { Button } from '@/components/ui/button'
import { useConnectionStatus } from '@/hooks/useConnectionStatus.ts'
import { cn } from '@/lib/utils'
import { Settings2Icon } from 'lucide-react'
import { NavLink } from 'react-router-dom'
interface Props {
  onOpenSettings: () => void
}

export function NavBar({ onOpenSettings }: Props) {
  const status = useConnectionStatus()

  const linkClass = ({ isActive }: { isActive: boolean }) =>
    cn(
      'px-3 py-1 text-sm rounded transition-colors',
      isActive
        ? 'bg-neutral-700 text-white'
        : 'text-neutral-400 hover:text-white hover:bg-neutral-800',
    )

  return (
    <header className="h-12 flex items-center px-4 gap-4 border-b border-neutral-800 bg-neutral-950 shrink-0">
      <span className="font-mono font-semibold text-sm text-white mr-2 select-none">
        OpenSandbox
      </span>

      <nav className="flex items-center gap-1">
        <NavLink to="/" end className={linkClass}>
          Dashboard
        </NavLink>
        <NavLink to="/pools" className={linkClass}>
          Pools
        </NavLink>
      </nav>

      <div className="flex-1" />

      <div className="flex items-center gap-1.5 text-xs text-neutral-400">
        <span
          className={cn(
            'size-2 rounded-full',
            status === null
              ? 'bg-neutral-600'
              : status
                ? 'bg-green-500'
                : 'bg-red-500',
          )}
        />
        <span className="hidden sm:inline">
          {status === null ? 'Not connected' : status ? 'Connected' : 'Unreachable'}
        </span>
      </div>

      <Button
        variant="ghost"
        size="icon"
        onClick={onOpenSettings}
        title="Settings"
        className="size-8 text-muted-foreground hover:text-foreground"
      >
        <Settings2Icon />
      </Button>
    </header>
  )
}
