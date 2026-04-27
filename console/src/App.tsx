import { useState, useEffect } from 'react'
import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

import { Layout } from './components/Layout.tsx'
import { DashboardPage } from './pages/DashboardPage.tsx'
import { PoolsPage } from './pages/PoolsPage.tsx'
import { SettingsPanel } from './components/SettingsPanel.tsx'
import { Toaster } from './components/ui/sonner.tsx'
import { TooltipProvider } from './components/ui/tooltip.tsx'
import { useSettings } from './hooks/useSettings.ts'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      staleTime: 5_000,
    },
  },
})

export function App() {
  const { settings, updateSettings } = useSettings()
  const [showSettings, setShowSettings] = useState(false)

  // Auto-open settings when no server URL configured
  useEffect(() => {
    if (!settings.serverUrl) {
      setShowSettings(true)
    }
  }, [settings.serverUrl])

  return (
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <BrowserRouter>
          <Routes>
            <Route
              element={<Layout onOpenSettings={() => setShowSettings(true)} />}
            >
              <Route index element={<DashboardPage />} />
              <Route path="pools" element={<PoolsPage />} />
            </Route>
          </Routes>

          {showSettings && (
            <SettingsPanel
              initial={settings}
              onSave={updateSettings}
              onClose={() => setShowSettings(false)}
            />
          )}

          <Toaster />
        </BrowserRouter>
      </TooltipProvider>
    </QueryClientProvider>
  )
}
