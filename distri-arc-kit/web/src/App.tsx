import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useState } from 'react'
import { RouterProvider } from 'react-router'
import { SseProvider } from './app/SseProvider'
import { router } from './app/router'

export default function App() {
  const [qc] = useState(() => new QueryClient({ defaultOptions: { queries: { staleTime: 15_000, refetchOnWindowFocus: false, retry: 1 } } }))
  return (
    <QueryClientProvider client={qc}>
      <SseProvider>
        <RouterProvider router={router} />
      </SseProvider>
    </QueryClientProvider>
  )
}
