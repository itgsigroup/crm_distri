import { describe, expect, it } from 'vitest'
import { ContentError, guardText, toMessage } from '../src/content.js'
import { messageText } from '../src/events.js'

const noConvert = async (b: Buffer) => Buffer.concat([Buffer.from('OGG:'), b])

describe('content to send', () => {
  it('builds photos, documents and voice notes (converted to ogg/opus)', async () => {
    const img = await toMessage({ kind: 'image', mime: 'image/jpeg', data: Buffer.from('JPEG').toString('base64') }, 'Foto stok', noConvert)
    expect(img).toMatchObject({ mimetype: 'image/jpeg', caption: 'Foto stok' })
    const doc = await toMessage({ kind: 'document', mime: 'application/pdf', file_name: 'PO.pdf', data: 'JVBERg==' }, '', noConvert)
    expect(doc).toMatchObject({ fileName: 'PO.pdf', mimetype: 'application/pdf' })
    const vn = (await toMessage({ kind: 'audio', ptt: true, seconds: 4, mime: 'audio/webm;codecs=opus', data: Buffer.from('WEBM').toString('base64') }, '', noConvert)) as { audio: Buffer; ptt: boolean; mimetype: string }
    expect(vn.ptt).toBe(true)
    expect(vn.mimetype).toBe('audio/ogg; codecs=opus')
    expect(vn.audio.toString()).toBe('OGG:WEBM')
  })
  it('builds polls and refuses bad ones', async () => {
    expect(await toMessage({ kind: 'poll', poll: { name: 'Kirim kapan?', options: ['Senin', ' Selasa ', ''], multi: true } }, '', noConvert))
      .toEqual({ poll: { name: 'Kirim kapan?', values: ['Senin', 'Selasa'], selectableCount: 2 } })
    await expect(toMessage({ kind: 'poll', poll: { name: 'x', options: ['satu'] } }, '', noConvert)).rejects.toBeInstanceOf(ContentError)
    await expect(toMessage({ kind: 'image', data: '' }, '', noConvert)).rejects.toBeInstanceOf(ContentError)
  })
  it('compares polls and files by their content for the identical-text guard', () => {
    expect(guardText({ kind: 'poll', poll: { name: 'Q', options: [] } }, '')).toBe('poll:Q')
    expect(guardText(undefined, 'halo')).toBe('halo')
  })
  it('shows an incoming poll as its question and options', () => {
    expect(messageText({ pollCreationMessageV3: { name: 'Jadwal kunjungan?', options: [{ optionName: 'Senin' }, { optionName: 'Rabu' }] } }))
      .toMatchObject({ text: 'Jadwal kunjungan?\n• Senin\n• Rabu', media: { kind: 'poll' } })
  })
})
