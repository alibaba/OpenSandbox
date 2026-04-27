import { Outlet } from 'react-router-dom'
import { NavBar } from './NavBar.tsx'

interface Props {
  onOpenSettings: () => void
}

export function Layout({ onOpenSettings }: Props) {
  return (
    <div className="flex flex-col h-screen bg-neutral-950 text-white overflow-hidden">
      <NavBar onOpenSettings={onOpenSettings} />
      <main className="flex-1 overflow-auto">
        <Outlet />
      </main>
    </div>
  )
}
