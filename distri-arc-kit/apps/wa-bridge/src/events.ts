import { isJidBroadcast, isJidGroup, isJidNewsletter, isJidStatusBroadcast, isLidUser, normalizeMessageContent, type WAMessage, type proto } from 'baileys'

// WaEvent is the bridge → worker contract; Go side: internal/wa/baileys.go (bridgeEvent).
export interface GroupMember { jid: string; phone: string; name: string }
export interface GroupMeta { name: string; members: GroupMember[] }
export interface MediaMeta { kind: string; mime: string; file_name: string; size: number }
export interface WaEvent {
  wamid: string
  session: string
  from: string
  to: string
  chat_id: string
  is_group: boolean
  group_meta?: GroupMeta
  sender_name: string
  text: string
  media_meta?: MediaMeta
  timestamp: string
  quoted?: string
  from_me: boolean
  is_history: boolean
  chat_name?: string // saved contact / business / push name of a direct chat
  transport: 'bridge'
}

/** Resolves LIDs to phone JIDs and group metadata for the mapping. */
export interface Resolver {
  ownPhone: string
  pnForLid(lid: string): Promise<string | null>
  groupMeta(jid: string): Promise<GroupMeta | undefined>
}

const digits = (jid: string | null | undefined): string => (jid ?? '').split('@')[0].split(':')[0].replace(/\D/g, '')

const size = (v: unknown): number => (v == null ? 0 : Number(v))

/** Extracts human text and media metadata. Media content is never downloaded. */
export function messageText(message: proto.IMessage | null | undefined): { text: string; media?: MediaMeta; quoted?: string } {
  const m = normalizeMessageContent(message)
  if (!m) return { text: '' }
  const quoted = m.extendedTextMessage?.contextInfo?.stanzaId ?? m.imageMessage?.contextInfo?.stanzaId ?? undefined
  if (m.conversation) return { text: m.conversation, quoted }
  if (m.extendedTextMessage) return { text: m.extendedTextMessage.text ?? '', quoted }
  if (m.imageMessage) return { text: m.imageMessage.caption ?? '', media: { kind: 'image', mime: m.imageMessage.mimetype ?? '', file_name: '', size: size(m.imageMessage.fileLength) }, quoted }
  if (m.documentMessage) return { text: m.documentMessage.caption ?? '', media: { kind: 'document', mime: m.documentMessage.mimetype ?? '', file_name: m.documentMessage.fileName ?? '', size: size(m.documentMessage.fileLength) }, quoted }
  if (m.videoMessage) return { text: m.videoMessage.caption ?? '', media: { kind: 'video', mime: m.videoMessage.mimetype ?? '', file_name: '', size: size(m.videoMessage.fileLength) }, quoted }
  if (m.audioMessage) return { text: '', media: { kind: 'audio', mime: m.audioMessage.mimetype ?? '', file_name: '', size: size(m.audioMessage.fileLength) }, quoted }
  if (m.locationMessage) return { text: m.locationMessage.name ?? '', media: { kind: 'location', mime: '', file_name: '', size: 0 } }
  if (m.contactMessage) return { text: m.contactMessage.displayName ?? '', media: { kind: 'contact', mime: '', file_name: '', size: 0 } }
  return { text: '' }
}

/** Whether a chat is a customer/group conversation ARC may look at (not status, channels or broadcast lists). */
export function isConversation(jid: string | null | undefined): jid is string {
  return !!jid && !isJidStatusBroadcast(jid) && !isJidBroadcast(jid) && !isJidNewsletter(jid)
}

async function toPhoneJid(r: Resolver, jid: string, alt?: string | null): Promise<string> {
  if (!isLidUser(jid)) return jid
  if (alt && !isLidUser(alt)) return alt
  return (await r.pnForLid(jid)) ?? jid
}

/** Maps a Baileys message to a WaEvent, or null for protocol messages, reactions and status updates. */
export async function toWaEvent(session: string, msg: WAMessage, r: Resolver, history: boolean): Promise<WaEvent | null> {
  const key = msg.key
  if (!key?.id || !isConversation(key.remoteJid) || !msg.message) return null
  const { text, media, quoted } = messageText(msg.message)
  if (!text && !media) return null
  const group = !!isJidGroup(key.remoteJid)
  const chat = group ? key.remoteJid : await toPhoneJid(r, key.remoteJid, key.remoteJidAlt)
  const fromMe = !!key.fromMe
  let from: string
  if (fromMe) from = r.ownPhone
  else if (group) from = digits(await toPhoneJid(r, key.participant ?? '', key.participantAlt))
  else from = digits(chat)
  const ts = typeof msg.messageTimestamp === 'number' ? msg.messageTimestamp : Number(msg.messageTimestamp ?? 0)
  const ev: WaEvent = {
    wamid: key.id,
    session,
    from,
    to: fromMe ? (group ? '' : digits(chat)) : r.ownPhone,
    chat_id: chat,
    is_group: group,
    sender_name: fromMe ? '' : (msg.pushName ?? ''),
    text,
    timestamp: new Date((ts || Date.now() / 1000) * 1000).toISOString(),
    from_me: fromMe,
    is_history: history,
    transport: 'bridge',
  }
  if (media) ev.media_meta = media
  if (quoted) ev.quoted = quoted
  if (group) ev.group_meta = (await r.groupMeta(chat)) ?? { name: '', members: [] }
  return ev
}

/** Group metadata in the API's shape; participants are resolved to phone numbers when known. */
export function toGroupMeta(subject: string, participants: { id: string; phoneNumber?: string; name?: string; notify?: string }[]): GroupMeta {
  return {
    name: subject,
    members: participants.map(p => ({ jid: p.id, phone: digits(p.phoneNumber ?? (isLidUser(p.id) ? '' : p.id)), name: p.name ?? p.notify ?? '' })),
  }
}
