import { createBrowserRouter } from 'react-router'
import { Shell } from './Shell'
import { FeedbackProvider } from '../components/feedback'
import { ControlCenter } from '../features/control/ControlCenter'
import { OrbitPage } from '../features/orbit/OrbitPage'
import { SegmenPage } from '../features/segmen/SegmenPage'
import { DealerPage } from '../features/dealer/DealerPage'
import { Upcoming } from '../features/placeholder/Upcoming'
import { ChatPage } from '../features/chat/ChatPage'
import { SettingsPage } from '../features/settings/SettingsPage'

// Routes of docs/design/08-frontend.md.
export const router = createBrowserRouter([
  {
    path: '/',
    // inside the router so sheets can navigate (provenance chips, "Buka dealer")
    element: <FeedbackProvider><Shell /></FeedbackProvider>,
    children: [
      { index: true, element: <ControlCenter /> },
      { path: 'orchestrator', element: <Upcoming title="Orchestrator" stage="06" text="Siklus per jam enam tahap (Ingest → Analisis → Sintesis & konflik → Keputusan → Eksekusi → Belajar), resolusi konflik antar agen, riwayat analisis, kartu agen, dan matriks otonomi." /> },
      { path: 'chat/:threadId?', element: <ChatPage /> },
      { path: 'orbit', element: <OrbitPage /> },
      { path: 'orbit/segmen', element: <SegmenPage /> },
      { path: 'orbit/relasi', element: <Upcoming title="Peta relasi 3D" stage="08" text="Graph nomor sales ↔ dealer dari interaksi WhatsApp + order per periode 30–180 hari, pasangan terkuat, dan pola relasi." /> },
      { path: 'dealer/:id?', element: <DealerPage /> },
      { path: 'stok', element: <Upcoming title="Push stok" stage="09" text="KPI stok, kandidat push per SKU dengan floor margin, stok kritis dengan usulan transfer / PO, dan penjualan per produk." /> },
      { path: 'kredit', element: <Upcoming title="Kredit · kas" stage="09" text="DSO, piutang, lewat tempo, sisa limit tiap dealer, exposure vs limit, dan prediksi kas masuk 30 hari tertimbang pola bayar." /> },
      { path: 'pengaturan', element: <SettingsPage /> },
      { path: 'panduan', element: <Upcoming title="Panduan Orbit" stage="11" text="Satu gambar, 14 istilah, cara baca papan — konsep Orbit untuk tim." /> },
    ],
  },
])
