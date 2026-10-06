import { createBrowserRouter } from 'react-router'
import { Shell } from './Shell'
import { LoginPage } from '../features/login/LoginPage'
import { FeedbackProvider } from '../components/feedback'
import { OrchProvider } from './orch'
import { OrchestratorPage } from '../features/orchestrator/OrchestratorPage'
import { RelasiPage } from '../features/relasi/RelasiPage'
import { ControlCenter } from '../features/control/ControlCenter'
import { OrbitPage } from '../features/orbit/OrbitPage'
import { SegmenPage } from '../features/segmen/SegmenPage'
import { DealerPage } from '../features/dealer/DealerPage'
import { ChatPage } from '../features/chat/ChatPage'
import { StockPage } from '../features/stock/StockPage'
import { GuidePage } from '../features/guide/GuidePage'
import { CreditPage } from '../features/credit/CreditPage'
import { SettingsPage } from '../features/settings/SettingsPage'

// Routes of docs/design/08-frontend.md.
export const router = createBrowserRouter([
  { path: '/login', element: <LoginPage /> },
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
      { path: 'panduan', element: <GuidePage /> },
    ],
  },
])
