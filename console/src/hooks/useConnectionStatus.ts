import { useState, useEffect } from 'react'
import { getConnectionStatus, onConnectionStatusChange } from '@/api/client.ts'

export function useConnectionStatus(): boolean | null {
  const [status, setStatus] = useState<boolean | null>(getConnectionStatus)

  useEffect(() => {
    return onConnectionStatusChange(setStatus)
  }, [])

  return status
}
