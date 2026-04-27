interface Props {
  rows?: number
  className?: string
}

export function SkeletonRow({ className = '' }: { className?: string }) {
  return <div className={`h-8 bg-neutral-800 rounded animate-pulse ${className}`} />
}

export function SkeletonTable({ rows = 4 }: Props) {
  return (
    <div className="space-y-2 p-4">
      {Array.from({ length: rows }).map((_, i) => (
        <SkeletonRow key={i} className="w-full" />
      ))}
    </div>
  )
}
