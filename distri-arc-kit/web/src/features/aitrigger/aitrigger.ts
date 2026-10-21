import type { ReplyAction, ReplyRule } from '../../api/types'

// AI Trigger (ADR 0041): the kinds of answer, their wording on the page, and a new rule's defaults.

export interface ActionInfo {
  label: string
  icon: string
  what: string
  ready: string // label of template_ready
  empty: string // label of template_empty
  placeholders: Record<string, string>
  defaults: Pick<ReplyRule, 'name' | 'keywords' | 'template_ready' | 'template_empty'>
}

export const ACTIONS: Record<ReplyAction, ActionInfo> = {
  stock: {
    label: 'Cek stok', icon: 'box', what: 'Stok produk yang disebut di pesan, per cabang',
    ready: 'Draf bila stok ready', empty: 'Draf bila stok kosong',
    placeholders: { produk: 'nama produk', jumlah: 'stok di cabang chat', cabang: 'cabang chat', cabang_lain: 'cabang lain yang ada stoknya', harga: 'harga sesuai tier dealer (tanpa "Rp"; kosong bila belum ada)' },
    defaults: { name: 'Tanya stok', keywords: ['stok', 'ready', 'ada barang', 'tersedia'], template_ready: 'Ready {jumlah} pcs untuk {produk} di {cabang}.', template_empty: 'Mohon maaf, {produk} di {cabang} sedang kosong.' },
  },
  price: {
    label: 'Cek harga', icon: 'tag', what: 'Harga produk yang disebut di pesan, sesuai tier dealer',
    ready: 'Draf bila harga ada', empty: 'Draf bila harga belum ada',
    placeholders: { produk: 'nama produk', harga: 'harga sesuai tier dealer (tanpa "Rp")' },
    defaults: { name: 'Tanya harga', keywords: ['harga', 'price', 'pricelist'], template_ready: 'Harga {produk} Rp {harga} per unit.', template_empty: 'Mohon maaf, harga {produk} kami cek dulu ya, segera kami kabari.' },
  },
  credit: {
    label: 'Limit & tagihan', icon: 'cash', what: 'Sisa limit kredit dan tagihan terbuka dealer di chat',
    ready: 'Draf bila kredit aman', empty: 'Draf bila ada tagihan lewat tempo / melewati limit',
    placeholders: { limit: 'limit kredit', sisa_limit: 'sisa limit', tagihan: 'total tagihan terbuka', jumlah_tagihan: 'jumlah invoice terbuka', jatuh_tempo: 'jatuh tempo tagihan tertua' },
    defaults: { name: 'Tanya limit & tagihan', keywords: ['limit', 'sisa limit', 'tagihan', 'jatuh tempo'], template_ready: 'Sisa limit saat ini Rp {sisa_limit} dari limit Rp {limit}.', template_empty: 'Saat ini ada {jumlah_tagihan} tagihan terbuka senilai Rp {tagihan}, tertua jatuh tempo {jatuh_tempo}. Mohon dibantu pelunasannya ya.' },
  },
  order: {
    label: 'Status order', icon: 'kanban', what: 'Status order terakhir dealer di chat (order → siap → kirim → invoice → bayar)',
    ready: 'Draf bila ada order', empty: 'Draf bila belum ada order',
    placeholders: { nomor: 'nomor SO', tanggal: 'tanggal order', status: 'status (mis. "dalam pengiriman")', total: 'total order (tanpa "Rp")' },
    defaults: { name: 'Status order', keywords: ['status order', 'pesanan saya', 'kapan dikirim', 'resi'], template_ready: 'Order {nomor} tanggal {tanggal} saat ini {status}.', template_empty: 'Mohon maaf, pesanan Bapak/Ibu kami cek dulu ya.' },
  },
}

export const ACTION_ORDER: ReplyAction[] = ['stock', 'price', 'credit', 'order']

/** Splits "stok, ready ,, ada barang" into clean keywords (lowercase, no duplicates). */
export function parseKeywords(s: string): string[] {
  const out: string[] = []
  for (const k of s.split(/[,\n]/)) {
    const v = k.trim().toLowerCase()
    if (v && !out.includes(v)) out.push(v)
  }
  return out
}

/** The {placeholders} of a template that the action does not fill (shown as a warning). */
export function unknownPlaceholders(tpl: string, action: ReplyAction): string[] {
  const known = Object.keys(ACTIONS[action].placeholders)
  return [...tpl.matchAll(/\{([a-z_]+)\}/g)].map((m) => m[1]).filter((p, i, a) => !known.includes(p) && a.indexOf(p) === i)
}
