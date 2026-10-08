// All user-facing copy, taken from the approved mockup (reference/distri-arc-orbit-v2-mockup.html).
// Terms follow docs/design/01-glossary.md exactly.
import type { RootCause, Segment } from '../../api/types'

export type ScreenKey = 'today' | 'dashboard' | 'orch' | 'chat' | 'orbit' | 'net' | 'kuad' | 'dealer' | 'stock' | 'ar' | 'users' | 'roles' | 'branches' | 'mcp' | 'conn' | 'konsep'

export const TITLES: Record<ScreenKey, [string, string]> = {
  today: ['Pusat kendali', ''],
  dashboard: ['Dashboard', 'Ringkasan kondisi dealer, order, kas, stok, dan aktivitas'],
  orch: ['Orchestrator', 'Mengatur enam agen AI: sinyal → analisis → konflik → keputusan → eksekusi → belajar'],
  chat: ['Chat', 'WhatsApp banyak nomor (Baileys) + grup gudang · order masuk dari sini'],
  orbit: ['Orbit', 'Semua dealer menurut siklus ordernya · atas = jadwal order · keluar = lewat jadwal'],
  net: ['Orbit', 'Peta relasi: kedekatan nomor sales ↔ dealer dalam 3D'],
  kuad: ['Orbit', 'Segmen: dealer dikelompokkan menurut seberapa sering dan seberapa besar ordernya'],
  dealer: ['Dealer', 'Frekuensi order, order-to-cash, share of wallet, product mix, sisa limit, PIC aktif — satu halaman per dealer'],
  stock: ['Push stok', 'Perputaran stok: stok yang menua menjadi alasan follow-up dealer yang jadwal order'],
  ar: ['Sisa limit', 'DSO (order → bayar): ruang kredit tiap dealer, pola bayar, prediksi kas masuk'],
  users: ['Pengguna', 'Master pengguna: akun login, peran, dan cabang · nomor WhatsApp ditautkan di Chat'],
  roles: ['Peran & akses', 'Master peran: centang halaman, cakupan data, dan keputusan yang boleh tiap peran'],
  branches: ['Cabang', 'Master cabang: dipakai pengguna, dealer, dan mapping data'],
  mcp: ['MCP Claude', 'Hubungkan Claude untuk menganalisis semua data GSI Orbit — Claude membaca dan mengusulkan, keputusan tetap di aplikasi'],
  conn: ['Pengaturan', 'Kebijakan orbit, sumber sinyal, koneksi AI (API & MCP), kalibrasi agen'],
  konsep: ['Konsep orbit', 'Panduan Orbit: satu gambar, 14 istilah, cara baca papan'],
}

export const AGENT_NAMES = ['AI Order', 'AI Follow-up', 'AI Kredit', 'AI Stok', 'AI Penagihan', 'AI Prospek']

export const PIPELINE_STAGES = ['Ingest', 'Analisis', 'Sintesis', 'Keputusan', 'Eksekusi', 'Belajar']
export const STAGE_LABEL: Record<string, string> = { ingest: 'Ingest', analyze: 'Analisis', synthesize: 'Sintesis', decide: 'Keputusan', execute: 'Eksekusi', learn: 'Belajar' }

export const KAT = ['Kamera & NVR', 'HDD & storage', 'Kabel & PoE', 'Modul LED', 'Fire alarm', 'Aksesoris']

/** n = glossary name; nick + todo = plain words for the Segmen boxes (readable without knowing the glossary). */
export interface SegmentInfo { n: string; nick: string; todo: string; s: string; k: string; play: string; desc: string; agent: string; risk: string }

export const KUAD: Record<Segment, SegmentInfo> = {
  A: { n: 'Segmen A', nick: 'Pelanggan andalan', todo: 'Jaga baik-baik: stok & respons nomor satu', s: 'sering × besar', k: 'accent', play: 'Prioritas: jaga & layani terbaik', desc: 'Tulang punggung omzet. Prioritas stok dan kecepatan respons, limit kredit tumbuh mengikuti siklus order, minimal 2 PIC aktif, review harga tier setahun sekali — bukan tiap nego.', agent: 'AI Follow-up + AI Kredit', risk: 'Konsentrasi: sedikit dealer menopang sebagian besar omzet. Satu yang lewat jadwal langsung terasa di kas.' },
  B: { n: 'Segmen B', nick: 'Rutin tapi kecil', todo: 'Ajak belanja lebih banyak per order', s: 'sering × kecil', k: 'good', play: 'Upsell: naikkan nilai order', desc: 'Order rutin tapi kecil, biaya layani per order tinggi. Bundle, minimum order bebas ongkir, konsolidasi kiriman mingguan, tawarkan kategori produk yang belum pernah dibeli. Target: naik ke Segmen A.', agent: 'AI Stok', risk: 'Margin tergerus ongkir dan admin selama nilai per order tidak naik.' },
  C: { n: 'Segmen C', nick: 'Pelanggan proyek', todo: 'Tanya proyek berikutnya, siapkan limit', s: 'jarang × besar', k: 'indigo', play: 'Project-based: ikuti proyeknya', desc: 'Order datang bersama proyek. Tanya pipeline proyek tiap kuartal, siapkan limit kredit (limit sementara / DP) dan lead time stok sebelum proyek berikutnya. Jangan di-follow-up berdasarkan siklus order.', agent: 'AI Order + AI Kredit', risk: 'Over limit saat proyek belum cair — piutang besar dalam sekali jalan.' },
  D: { n: 'Segmen D', nick: 'Kecil & jarang', todo: 'Cukup dilayani otomatis lewat WA', s: 'jarang × kecil', k: 'text-3', play: 'Low-touch: layani otomatis', desc: 'Cash / transfer, harga tier C, katalog WA self-service, follow-up otomatis sebulan sekali, tanpa kunjungan. Naik ke Segmen B bila mulai rutin.', agent: 'AI Prospek (otomatis)', risk: 'Perhatian sales lebih mahal dari marginnya.' },
  Prospek: { n: 'Prospek', nick: 'Belum pernah order', todo: 'Ajak order pertama', s: 'belum pernah order', k: 'text-3', play: 'Aktifkan order pertama', desc: 'Pelanggan terdaftar di Accurate tapi belum pernah order dalam 18 bulan data. Tidak digambar di Orbit dan Segmen; digarap AI Prospek dan sales cabangnya.', agent: 'AI Prospek', risk: '' },
  Baru: { n: 'Baru', nick: 'Dealer baru', todo: 'Tunggu order kedua', s: 'order pertama ≤ 90 hari', k: 'text-3', play: 'Amati dua putaran', desc: 'Order pertama dalam 90 hari terakhir, belum membentuk siklus. Ditempatkan sementara dari nilai order pertama; segmen ditetapkan setelah order kedua.', agent: 'AI Prospek', risk: '' },
}

export const SEGMENT_ORDER: Segment[] = ['A', 'B', 'C', 'D', 'Baru']

export const RING_DESC: Record<string, string> = {
  'Key account': 'omzet besar, siklus order terjaga, bayar tepat waktu',
  Aktif: 'di dalam siklus order',
  'At risk': '> 1,2× siklus order',
  Churn: '> 2× siklus order, sekali order lalu hilang, atau diam > 1 tahun',
}

/** Akar terduga of a drifting dealer (AI Follow-up). */
export const ROOT_CAUSE: Record<RootCause, string> = {
  project_unpaid: 'Akar terduga: proyek belum cair (minta tempo)',
  marketplace_module: 'Akar terduga: harga modul vs marketplace',
  marketplace: 'Akar terduga: harga vs marketplace',
  wholesaler: 'Akar terduga: beralih ke grosir lokal · WA tak dibalas',
  small_share: 'Share of wallet kecil sejak awal · prioritas rendah',
}

/** Short root cause used in the brief ("akar: …"). */
export const ROOT_SHORT: Record<RootCause, string> = {
  project_unpaid: 'proyek belum cair',
  marketplace_module: 'harga modul vs marketplace',
  marketplace: 'harga vs marketplace',
  wholesaler: 'beralih ke grosir lokal',
  small_share: 'porsi kecil',
}

export const PENDING_ORCH = 'Belum ada saran untuk ini — jalankan Analisis ulang'
