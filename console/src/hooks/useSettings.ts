import { useState, useCallback } from 'react'
import { loadSettings, saveSettings } from '@/api/client.ts'
import type { Settings } from '@/api/client.ts'

export function useSettings() {
  const [settings, setSettingsState] = useState<Settings>(loadSettings)

  const updateSettings = useCallback((next: Settings) => {
    saveSettings(next)
    setSettingsState(next)
  }, [])

  return { settings, updateSettings }
}
