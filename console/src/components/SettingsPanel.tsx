import { useState } from 'react'
import type { Settings } from '@/api/client.ts'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'

interface Props {
  initial: Settings
  onSave: (settings: Settings) => void
  onClose: () => void
}

export function SettingsPanel({ initial, onSave, onClose }: Props) {
  const [serverUrl, setServerUrl] = useState(initial.serverUrl)
  const [authToken, setAuthToken] = useState(initial.authToken)

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!serverUrl.trim()) return
    onSave({ serverUrl: serverUrl.trim(), authToken: authToken.trim() })
    onClose()
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Connection Settings</DialogTitle>
          <DialogDescription>
            Settings are stored locally in your browser and never sent to a third party.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="serverUrl">
              OpenSandbox Server URL{' '}
              <span className="text-destructive">*</span>
            </Label>
            <Input
              id="serverUrl"
              type="url"
              value={serverUrl}
              onChange={(e) => setServerUrl(e.target.value)}
              placeholder="http://localhost:8090"
              required
            />
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="authToken">
              Bearer Token{' '}
              <span className="text-muted-foreground">(optional)</span>
            </Label>
            <Input
              id="authToken"
              type="password"
              value={authToken}
              onChange={(e) => setAuthToken(e.target.value)}
              placeholder="sk-..."
            />
          </div>

          <div className="flex gap-3 justify-end pt-2">
            {initial.serverUrl && (
              <Button type="button" variant="outline" onClick={onClose}>
                Cancel
              </Button>
            )}
            <Button type="submit">Save</Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  )
}
