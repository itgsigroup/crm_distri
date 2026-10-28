// Tempel (paste) di kolom chat, like WhatsApp Web (pure, tested): a copied screenshot or picture becomes the file to
// send; copied text stays text; copied spreadsheet cells (Excel) are sent as the picture of the table.

/** Cells copied from a spreadsheet: Excel's HTML (or another spreadsheet's), or tab-separated rows. Excel puts a
 * picture of the cells beside the text; WhatsApp Web sends that picture (Sam 28 Oct). */
export function fromSpreadsheet(html: string, text: string): boolean {
  if (/urn:schemas-microsoft-com:office:excel|ProgId content=["']?Excel|google-sheets-html-origin|LibreOffice|x:str/i.test(html)) return true
  return /\t/.test(text) && /<table/i.test(html)
}

/** The picture to send from a paste, or null when the paste is text (or nothing to send). A paste carrying text
 * (from a document or a web page) stays text, unless it is spreadsheet cells with their picture; a screenshot or a
 * copied picture comes as an image file only. */
export function pastedImage(files: ArrayLike<File>, text: string, now: Date = new Date(), html = ''): File | null {
  if (text.trim() && !fromSpreadsheet(html, text)) return null
  const img = Array.from(files).find((f) => f.type.startsWith('image/'))
  if (!img) return null
  // a screenshot comes as "image.png": give it a name worth reading on the customer's phone
  if (!img.name || /^image\.\w+$/i.test(img.name)) {
    const ext = (img.type.split('/')[1] || 'png').replace('jpeg', 'jpg').replace(/\W.*$/, '')
    const stamp = now.toISOString().slice(0, 19).replace(/[-:]/g, '').replace('T', '-')
    return new File([img], `${text.trim() ? 'tabel' : 'gambar'}-${stamp}.${ext}`, { type: img.type })
  }
  return img
}
