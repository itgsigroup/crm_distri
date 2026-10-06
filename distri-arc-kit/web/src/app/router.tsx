import { createBrowserRouter } from 'react-router'
import { Shell } from './Shell'
import { FeedbackProvider } from '../components/feedback'
import { OrchProvider } from './orch'
import { OrchestratorPage } from '../features/orchestrator/OrchestratorPage'
import { RelasiPage } from '../features/relasi/RelasiPage'
import { ControlCenter } from '../features/control/ControlCenter'
import { OrbitPage } from '../features/orbit/OrbitPage'
import { SegmenPage } from '../features/segmen/SegmenPage'
import { DealerPage } from '../features/dealer/DealerPage'
import { Upcoming } from '../features/placeholder/Upcoming'
import { ChatPage } from '../features/chat/ChatPage'
import { StockPage } from '../features/stock/StockPage'
import { CreditPage } from '../features/credit/CreditPage'
import { SettingsPage } from '../features/settings/SettingsPage'

// Routes of docs/design/08-frontend.md.
export const router = createBrowserRouter([
  {
    path: '/',
    // inside the router so sheets can navigate (provenance chips, "Buka dealer")
    element: <FeedbackProvider><OrchProvider><Shell /></OrchProvider></FeedbackProvider>,
    children: [
      { index: true, element: <ControlCenter /> },
      { path: 'orchestrator', element: <OrchestratorPage /> },
      { path: 'chat/:threadId?', element: <ChatPage /> },
      { path: 'orbit', element: <OrbitPage /> },
      { path: 'orbit/segmen', element: <SegmenPage /> },
      { path: 'orbit/relasi', element: <RelasiPage /> },
      { path: 'dealer/:id?', element: <DealerPage /> },
      { path: 'stok', element: <StockPage /> },
      { path: 'kredit', element: <CreditPage /> },
      { path: 'pengaturan', element: <SettingsPage /> },
      { path: 'panduan', element: <Upcoming title="Panduan Orbit" stage="11" text="Satu gambar, 14 istilah, cara baca papan — konsep Orbit untuk tim." /> },
    ],
  },
])
