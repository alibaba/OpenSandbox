import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

interface Props {
  rows?: number
}

export function SkeletonRow({ className = '' }: { className?: string }) {
  return <Skeleton className={cn('h-8', className)} />
}

export function SkeletonTable({ rows = 4 }: Props) {
  return (
    <div className="flex flex-col gap-2 p-4">
      {Array.from({ length: rows }).map((_, i) => (
        <SkeletonRow key={i} className="w-full" />
      ))}
    </div>
  )
}
