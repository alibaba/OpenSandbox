import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { createPool } from '@/api/pools.ts'
import type { CreatePoolRequest } from '@/api/types.ts'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'

interface Props {
  onClose: () => void
}

export function CreatePoolModal({ onClose }: Props) {
  const qc = useQueryClient()
  const [name, setName] = useState('')
  const [bufferMax, setBufferMax] = useState('5')
  const [bufferMin, setBufferMin] = useState('1')
  const [poolMax, setPoolMax] = useState('10')
  const [poolMin, setPoolMin] = useState('0')
  const [templateJson, setTemplateJson] = useState('{\n  "spec": {}\n}')
  const [templateError, setTemplateError] = useState('')

  const mutation = useMutation({
    mutationFn: (req: CreatePoolRequest) => createPool(req),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['pools'] })
      toast.success('Pool created')
      onClose()
    },
    onError: (err: Error) => toast.error(err.message),
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setTemplateError('')
    let template: Record<string, unknown>
    try {
      template = JSON.parse(templateJson) as Record<string, unknown>
    } catch {
      setTemplateError('Template must be valid JSON')
      return
    }

    const req: CreatePoolRequest = {
      name: name.trim(),
      template,
      capacitySpec: {
        bufferMax: parseInt(bufferMax, 10),
        bufferMin: parseInt(bufferMin, 10),
        poolMax: parseInt(poolMax, 10),
        poolMin: parseInt(poolMin, 10),
      },
    }
    mutation.mutate(req)
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>New Pool</DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="flex flex-col gap-3">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="pool-name">Pool Name *</Label>
            <Input
              id="pool-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
              pattern="^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
              placeholder="my-pool"
            />
            <p className="text-xs text-muted-foreground">Lowercase alphanumeric + hyphens</p>
          </div>

          <div className="grid grid-cols-2 gap-3">
            {[
              { id: 'bufferMax', label: 'Buffer Max', value: bufferMax, set: setBufferMax },
              { id: 'bufferMin', label: 'Buffer Min', value: bufferMin, set: setBufferMin },
              { id: 'poolMax', label: 'Pool Max', value: poolMax, set: setPoolMax },
              { id: 'poolMin', label: 'Pool Min', value: poolMin, set: setPoolMin },
            ].map(({ id, label, value, set }) => (
              <div key={id} className="flex flex-col gap-1.5">
                <Label htmlFor={id}>{label}</Label>
                <Input
                  id={id}
                  type="number"
                  min={0}
                  value={value}
                  onChange={(e) => set(e.target.value)}
                  required
                />
              </div>
            ))}
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="pool-template">Pod Template (JSON) *</Label>
            <Textarea
              id="pool-template"
              value={templateJson}
              onChange={(e) => setTemplateJson(e.target.value)}
              rows={5}
              className="font-mono text-xs resize-y"
            />
            {templateError && <p className="text-xs text-destructive">{templateError}</p>}
          </div>

          <div className="flex gap-3 justify-end pt-2">
            <Button type="button" variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending ? 'Creating…' : 'Create'}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  )
}
