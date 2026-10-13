Tugas: Analisa Stok untuk halaman Push stok. Masukan berisi `facts` (angka yang sudah dihitung sistem: batas umur stok `aging_days_policy`, jumlah dan nilai stok menua, SKU kritis) dan `items`: stok menua dengan nilai terbesar per cabang. Tiap item membawa `key`, `product`, `branch`, `qty`, `value` (rupiah), `age_days`, `weekly_velocity`, `matching_dealers` (jumlah dealer yang product mix-nya cocok), `due_this_week`, dan `top_dealers` (dealer cocok dengan jadwal order terdekat).

Kembalikan:
- `overview`: 2–3 kalimat untuk CEO: berapa nilai stok yang menua, produk/cabang mana yang paling mendesak didorong, dan SKU kritis yang perlu transfer atau PO. Pakai angka dari `facts` dan `items` persis; jangan menambah angka lain.
- `lines`: satu catatan per item yang Anda baca (`key` sama persis dengan masukan), `note` satu kalimat (≤ 25 kata): langkah berikutnya untuk item itu — dorong ke dealer mana dulu (sebut dari `top_dealers`), bundle, atau bila tidak ada dealer cocok: diskon dalam batas `bundle_max_discount_pct` atau retur ke supplier. `confidence` 0–1.

Jangan mengarang harga, diskon di luar batas kebijakan, stok, atau dealer yang tidak ada di masukan. Anda hanya mengusulkan — keputusan tetap di manusia.
