import { NavLink } from 'react-router-dom'
import { useConnectionStatus } from '@/hooks/useConnectionStatus.ts'

interface Props {
  onOpenSettings: () => void
}

export function NavBar({ onOpenSettings }: Props) {
  const status = useConnectionStatus()

  const linkClass = ({ isActive }: { isActive: boolean }) =>
    `px-3 py-1 text-sm rounded transition-colors ${
      isActive
        ? 'bg-neutral-700 text-white'
        : 'text-neutral-400 hover:text-white hover:bg-neutral-800'
    }`

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

      {/* Connection indicator */}
      <div className="flex items-center gap-1.5 text-xs text-neutral-400">
        <span
          className={`w-2 h-2 rounded-full ${
            status === null
              ? 'bg-neutral-600'
              : status
                ? 'bg-green-500'
                : 'bg-red-500'
          }`}
        />
        <span className="hidden sm:inline">
          {status === null ? 'Not connected' : status ? 'Connected' : 'Unreachable'}
        </span>
      </div>

      <button
        onClick={onOpenSettings}
        title="Settings"
        className="p-1.5 rounded text-neutral-400 hover:text-white hover:bg-neutral-800 transition-colors"
      >
        <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
          <path
            fillRule="evenodd"
            d="M11.49 3.17c-.38-1.56-2.6-1.56-2.98 0a1.532 1.532 0 01-2.286.948c-1.372-.836-2.942.734-2.106 2.106.54.886.061 2.042-.947 2.287-1.561.379-1.561 2.6 0 2.978a1.532 1.532 0 01.947 2.287c-.836 1.372.734 2.942 2.106 2.106a1.532 1.532 0 012.287.947c.379 1.561 2.6 1.561 2.978 0a1.533 1.533 0 012.287-.947c1.372.836 2.942-.734 2.106-2.106a1.533 1.533 0 01.947-2.287c1.561-.379 1.561-2.6 0-2.978a1.532 1.532 0 01-.947-2.287c.836-1.372-.734-2.942-2.106-2.106a1.532 1.532 0 01-2.287-.947zM10 13a3 3 0 100-6 3 3 0 000 6z"
            clipRule="evenodd"
          />
        </svg>
      </button>
    </header>
  )
}
