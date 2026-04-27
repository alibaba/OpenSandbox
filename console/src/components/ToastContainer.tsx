import type { Toast } from '@/hooks/useToast.ts'

interface Props {
  toasts: Toast[]
  onDismiss: (id: string) => void
}

const variantClass = {
  success: 'bg-green-900/90 border-green-600 text-green-200',
  error: 'bg-red-900/90 border-red-600 text-red-200',
  info: 'bg-neutral-800/90 border-neutral-600 text-neutral-200',
}

export function ToastContainer({ toasts, onDismiss }: Props) {
  if (toasts.length === 0) return null
  return (
    <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2 w-80">
      {toasts.map((t) => (
        <div
          key={t.id}
          className={`flex items-start gap-3 px-4 py-3 rounded border text-sm shadow-lg ${variantClass[t.variant]}`}
        >
          <span className="flex-1">{t.message}</span>
          <button onClick={() => onDismiss(t.id)} className="opacity-60 hover:opacity-100 text-xs mt-0.5">
            ✕
          </button>
        </div>
      ))}
    </div>
  )
}
