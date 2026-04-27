interface Props {
  state: string
}

const stateColors: Record<string, string> = {
  Running: 'bg-green-500/20 text-green-400 border-green-500/40',
  Pending: 'bg-yellow-500/20 text-yellow-400 border-yellow-500/40',
  Pausing: 'bg-yellow-500/20 text-yellow-400 border-yellow-500/40',
  Paused: 'bg-blue-500/20 text-blue-400 border-blue-500/40',
  Stopping: 'bg-orange-500/20 text-orange-400 border-orange-500/40',
  Terminated: 'bg-neutral-500/20 text-neutral-400 border-neutral-500/40',
  Failed: 'bg-red-500/20 text-red-400 border-red-500/40',
}

export function StatusBadge({ state }: Props) {
  const cls = stateColors[state] ?? 'bg-neutral-500/20 text-neutral-400 border-neutral-500/40'
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-mono border ${cls}`}>
      {state}
    </span>
  )
}
