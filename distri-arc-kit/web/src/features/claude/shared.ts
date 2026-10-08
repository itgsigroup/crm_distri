// Shared by the MCP Claude page and the scheduled-analysis card.

export const SCOPE: Record<string, [string, string]> = {
  read: ['Baca', 'Dealer, jadwal order, kredit, stok, chat (nomor disamarkan), KPI, ringkasan'],
  analyze: ['Analisis', 'Analisis dealer, segmen, kas, stok — hasilnya usulan'],
  orchestrate: ['Orchestrator', 'Menjalankan siklus & mengirim usulan agen (butuh hak kebijakan)'],
  decide: ['Keputusan', 'Tidak pernah — setujui/tolak hanya oleh manusia di aplikasi'],
}

export const PROMPTS = [
  'Pakai Distri ARC: panggil data_ringkasan, lalu jelaskan kondisi bisnis distribusi saat ini — cabang dan sales mana yang perlu perhatian — dan beri 5 tindakan prioritas minggu ini.',
  'Dengan penjualan_bulanan 12 bulan per cabang, cabang mana yang tumbuh dan mana yang turun? Cari penyebabnya dari jadwal_lewat dan piutang_ringkas.',
  'Dari stok_aging (min_days 90) dan produk_terlaris 90 hari, susun rencana push stok per cabang: barang, dealer kandidat, dan urutan follow-up.',
  'Analisis piutang_ringkas: dealer mana yang berisiko macet, berapa prediksi kas 30 hari, dan dealer mana yang limitnya perlu ditinjau.',
  'Ambil 20 dealer Key account terbesar (dealer_list status "Key account", sort omzet), lihat product mix-nya dengan dealer_get, dan usulkan kategori yang bisa ditawarkan agar share of wallet naik.',
  'Bandingkan kinerja sales: dari data_ringkasan dan dealer_list per sales, siapa yang paling banyak dealer lewat jadwal, dan apa saran coaching-nya?',
]
