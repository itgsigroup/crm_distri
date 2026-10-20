import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { RouterProvider } from 'react-router'
import { SseProvider } from './app/SseProvider'
import { prefetchPages, router } from './app/router'

export default function App() {
  const [qc] = useState(() => new QueryClient({ defaultOptions: { queries: { staleTime: 15_000, refetchOnWindowFocus: false, retry: 1 } } }))
  useEffect(() => { prefetchPages() }, [])
  return (
    <QueryClientProvider client={qc}>
      <SseProvider>
        <RouterProvider router={router} />
      </SseProvider>
    </QueryClientProvider>
  )
}
