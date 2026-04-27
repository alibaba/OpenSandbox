import { useState, useEffect } from 'react'
import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

import { Layout } from './components/Layout.tsx'
import { DashboardPage } from './pages/DashboardPage.tsx'
import { PoolsPage } from './pages/PoolsPage.tsx'
import { SettingsPanel } from './components/SettingsPanel.tsx'
import { ToastContainer } from './components/ToastContainer.tsx'
import { useSettings } from './hooks/useSettings.ts'
import { useToast } from './hooks/useToast.ts'

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
  const { toasts, push: toast, dismiss } = useToast()
  const [showSettings, setShowSettings] = useState(false)

  // Auto-open settings when no server URL configured
  useEffect(() => {
    if (!settings.serverUrl) {
      setShowSettings(true)
    }
  }, [settings.serverUrl])

  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route
            element={<Layout onOpenSettings={() => setShowSettings(true)} />}
          >
            <Route
              index
              element={<DashboardPage toast={toast} />}
            />
            <Route
              path="pools"
              element={<PoolsPage toast={toast} />}
            />
          </Route>
        </Routes>

        {showSettings && (
          <SettingsPanel
            initial={settings}
            onSave={updateSettings}
            onClose={() => setShowSettings(false)}
          />
        )}

        <ToastContainer toasts={toasts} onDismiss={dismiss} />
      </BrowserRouter>
    </QueryClientProvider>
  )
}
