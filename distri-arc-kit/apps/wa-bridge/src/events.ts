import { isJidBroadcast, isJidGroup, isJidNewsletter, isJidStatusBroadcast, isLidUser, normalizeMessageContent, proto, WAMessageStubType, type WAMessage } from 'baileys'

// WaEvent is the bridge → worker contract; Go side: internal/wa/baileys.go (bridgeEvent).
export interface GroupMember { jid: string; phone: string; name: string; admin?: boolean }
/** admin_only: only admins may write (WhatsApp "announce") and this number is no admin — a send would be refused. */
export interface GroupMeta { name: string; members: GroupMember[]; admin_only?: boolean }
/**
 * Media of a message. The content is never downloaded on arrival; `ref` is what WhatsApp needs to fetch it later on
 * request (POST /sessions/{id}/media): the message key and the media's CDN path and decryption key, no thumbnail.
 */
/** A member tagged in a message ("@Budi"): the digits the text carries after "@" (a number or a LID), the member's
 * phone number when known, and the name the group knows them by. */
export interface Mention { token: string; phone: string; name: string }
export interface MediaMeta {
  kind: string; mime: string; file_name: string; size: number; seconds?: number; ptt?: boolean; ref?: string
  /** a WhatsApp call (kind "call", calls.ts): how it ended, and whether it was a video call */
  call?: 'missed' | 'answered' | 'rejected'; video?: boolean
  /** a group's own line (kind "system", groupStub): what changed, as WhatsApp words it */
  system?: string
}
export interface WaEvent {
  wamid: string
  session: string
  account: string // the linked phone's own number (sessions are named before the number is known)
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
  mentions?: Mention[] // members tagged in the text (WhatsApp "@")
  edit_of?: string // the message this one edits (its text replaces the original's)
  revoke_of?: string // the message this one deletes for everyone ("Pesan ini dihapus")
  react_to?: string // the message this one reacts to (text = the emoji, "" when taken back)
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

type Media = proto.Message.IImageMessage & proto.Message.IVideoMessage & proto.Message.IAudioMessage & proto.Message.IDocumentMessage & proto.Message.IStickerMessage

function meta(kind: string, x: Media): MediaMeta {
  const out: MediaMeta = { kind, mime: x.mimetype ?? '', file_name: x.fileName ?? '', size: size(x.fileLength) }
  if (x.seconds) out.seconds = x.seconds
  if (x.ptt) out.ptt = true
  return out
}

/** The contextInfo of a message's content (every content type carries one: the quoted message, the tagged members). */
function contextOf(m: proto.IMessage): proto.IContextInfo | undefined {
  for (const v of Object.values(m)) {
    if (v && typeof v === 'object' && 'contextInfo' in v) {
      const c = (v as { contextInfo?: proto.IContextInfo | null }).contextInfo
      if (c) return c
    }
  }
  return undefined
}

/** The JIDs tagged in a message ("@Budi"), as WhatsApp lists them. */
export function mentionedJids(message: proto.IMessage | null | undefined): string[] {
  const m = normalizeMessageContent(message)
  if (!m) return []
  return [...new Set((contextOf(m)?.mentionedJid ?? []).filter(Boolean))]
}

/** Extracts human text and media metadata. Media content is never downloaded here. */
export function messageText(message: proto.IMessage | null | undefined): { text: string; media?: MediaMeta; quoted?: string } {
  const m = normalizeMessageContent(message)
  if (!m) return { text: '' }
  // the message replied to ("Balas"): every content type carries it in its contextInfo
  const quoted = contextOf(m)?.stanzaId ?? undefined
  if (m.conversation) return { text: m.conversation, quoted }
  if (m.extendedTextMessage) return { text: m.extendedTextMessage.text ?? '', quoted }
  if (m.imageMessage) return { text: m.imageMessage.caption ?? '', media: meta('image', m.imageMessage), quoted }
  if (m.documentMessage) return { text: m.documentMessage.caption ?? '', media: meta('document', m.documentMessage), quoted }
  if (m.videoMessage) return { text: m.videoMessage.caption ?? '', media: meta(m.videoMessage.gifPlayback ? 'gif' : 'video', m.videoMessage), quoted }
  if (m.audioMessage) return { text: '', media: meta('audio', m.audioMessage), quoted }
  if (m.stickerMessage) return { text: '', media: meta('sticker', m.stickerMessage), quoted }
  const poll = m.pollCreationMessage ?? m.pollCreationMessageV2 ?? m.pollCreationMessageV3
  if (poll) {
    const opts = (poll.options ?? []).map((o) => o.optionName ?? '').filter(Boolean)
    return { text: [poll.name ?? '', ...opts.map((o) => '• ' + o)].join('\n'), media: { kind: 'poll', mime: '', file_name: '', size: 0 } }
  }
  if (m.locationMessage) return { text: m.locationMessage.name ?? '', media: { kind: 'location', mime: '', file_name: '', size: 0 } }
  if (m.contactMessage) {
    // the card's number (waid, else the first TEL) beside the name, so Chat shows whom the customer shared
    const card = m.contactMessage.vcard ?? ''
    const num = /waid=(\d+)/.exec(card)?.[1] ?? /TEL[^:]*:([+\d\s-]+)/.exec(card)?.[1]?.replace(/[^\d+]/g, '') ?? ''
    const name = m.contactMessage.displayName ?? ''
    return { text: [name, num && (num.startsWith('+') ? num : '+' + num)].filter(Boolean).join(' · '), media: { kind: 'contact', mime: '', file_name: '', size: 0 } }
  }
  return { text: '' }
}

const MEDIA_FIELDS = ['imageMessage', 'videoMessage', 'audioMessage', 'documentMessage', 'stickerMessage'] as const

/**
 * The download reference of a media message: a WebMessageInfo holding only the key and the media fields WhatsApp
 * needs to fetch and decrypt it (or to ask the phone to re-upload an expired file) — no caption, thumbnail or
 * context. Base64 of the protobuf; null for messages without downloadable media.
 */
export function mediaRef(msg: WAMessage): string | null {
  const m = normalizeMessageContent(msg.message)
  const field = MEDIA_FIELDS.find(f => m?.[f])
  if (!m || !field) return null
  const x = m[field] as Media
  if (!x.mediaKey || !(x.directPath || x.url)) return null
  const media: Media = { url: x.url, directPath: x.directPath, mediaKey: x.mediaKey, fileEncSha256: x.fileEncSha256, fileSha256: x.fileSha256, fileLength: x.fileLength, mimetype: x.mimetype, mediaKeyTimestamp: x.mediaKeyTimestamp }
  const k = msg.key
  const info = proto.WebMessageInfo.encode({ key: { remoteJid: k.remoteJid, fromMe: k.fromMe, id: k.id, participant: k.participant }, message: { [field]: media } }).finish()
  return Buffer.from(info).toString('base64')
}

/** Decodes a reference made by mediaRef back into a message Baileys can download. */
export function decodeMediaRef(ref: string): WAMessage {
  return proto.WebMessageInfo.decode(Buffer.from(ref, 'base64')) as WAMessage
}

/** A delete-for-everyone of an earlier message: the deleted message's id. */
export function revokeOf(message: proto.IMessage | null | undefined): string | null {
  const p = normalizeMessageContent(message)?.protocolMessage
  if (!p || p.type !== proto.Message.ProtocolMessage.Type.REVOKE || !p.key?.id) return null
  return p.key.id
}

/** A reaction (WhatsApp "Reaksi"): the reacted message's id and the emoji, "" when the reaction is taken back. */
export function reactionOf(message: proto.IMessage | null | undefined): { id: string; emoji: string } | null {
  const r = normalizeMessageContent(message)?.reactionMessage
  if (!r?.key?.id) return null
  return { id: r.key.id, emoji: r.text ?? '' }
}

/** An edit of an earlier message (WhatsApp "Edit"): the edited message's id and its new text. */
export function editOf(message: proto.IMessage | null | undefined): { id: string; text: string } | null {
  const p = normalizeMessageContent(message)?.protocolMessage
  if (!p || p.type !== proto.Message.ProtocolMessage.Type.MESSAGE_EDIT || !p.key?.id || !p.editedMessage) return null
  return { id: p.key.id, text: messageText(p.editedMessage).text }
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

/** The group changes WhatsApp shows as a line in the chat ("Grup dibuat", "Budi ditambahkan"): a stub message
 * without content. Each gets a name GSI Orbit stores with the line (media_meta.system). */
const GROUP_STUBS: Record<number, string> = {
  [WAMessageStubType.GROUP_CREATE]: 'create',
  [WAMessageStubType.GROUP_CHANGE_SUBJECT]: 'subject',
  [WAMessageStubType.GROUP_CHANGE_ICON]: 'icon',
  [WAMessageStubType.GROUP_CHANGE_DESCRIPTION]: 'description',
  [WAMessageStubType.GROUP_PARTICIPANT_ADD]: 'add',
  [WAMessageStubType.GROUP_PARTICIPANT_INVITE]: 'join',
  [WAMessageStubType.GROUP_PARTICIPANT_ADD_REQUEST_JOIN]: 'join',
  [WAMessageStubType.GROUP_PARTICIPANT_REMOVE]: 'remove',
  [WAMessageStubType.GROUP_PARTICIPANT_LEAVE]: 'leave',
  [WAMessageStubType.GROUP_PARTICIPANT_PROMOTE]: 'promote',
  [WAMessageStubType.GROUP_PARTICIPANT_DEMOTE]: 'demote',
}

/** The members a participant stub names: Baileys gives JSON ({id, phoneNumber}) or bare JIDs. */
export function stubParticipants(params: string[] | null | undefined): { jid: string; phone: string }[] {
  return (params ?? []).flatMap((p) => {
    if (p.startsWith('{')) {
      try {
        const o = JSON.parse(p) as { id?: string; phoneNumber?: string }
        if (!o.id) return []
        return [{ jid: o.id, phone: digits(o.phoneNumber ?? (isLidUser(o.id) ? '' : o.id)) }]
      } catch {
        return []
      }
    }
    return p.includes('@') ? [{ jid: p, phone: isLidUser(p) ? '' : digits(p) }] : []
  })
}

const fmtPhone = (d: string) => (d ? '+' + d : 'seseorang')

/** The line of a group change, as WhatsApp words it in Indonesian ("Anda menambahkan Budi"). */
export function groupStubLine(kind: string, who: string, names: string[], params: string[]): string {
  const list = names.join(', ') || 'seseorang'
  switch (kind) {
    case 'create': return `${who} membuat grup "${params[0] ?? ''}"`.replace(' ""', '')
    case 'subject': return `${who} mengubah nama grup menjadi "${params[0] ?? ''}"`
    case 'icon': return `${who} mengubah foto grup`
    case 'description': return `${who} mengubah deskripsi grup`
    case 'add': return `${who} menambahkan ${list}`
    case 'join': return `${list} bergabung`
    case 'remove': return `${who} mengeluarkan ${list}`
    case 'leave': return `${list} keluar`
    case 'promote': return `${list} sekarang admin`
    case 'demote': return `${list} bukan admin lagi`
  }
  return ''
}

/** A group stub (created, renamed, members added or gone) as a system line of the group's chat, so the chat exists
 * in GSI Orbit from the moment the group does, not from its first text (23 Oct: a group made that day stayed unseen). */
async function groupStub(session: string, msg: WAMessage, r: Resolver): Promise<WaEvent | null> {
  const key = msg.key
  const kind = GROUP_STUBS[msg.messageStubType ?? 0]
  if (!kind || !key?.id || !key.remoteJid) return null
  const chat = key.remoteJid
  const params = [...(msg.messageStubParameters ?? [])]
  const metaOf = (await r.groupMeta(chat)) ?? { name: '', members: [] }
  if (kind === 'create' && !params[0] && metaOf.name) params[0] = metaOf.name
  const actor = digits(await toPhoneJid(r, key.participant ?? '', key.participantAlt))
  const fromMe = !!key.fromMe || (!!actor && actor === r.ownPhone)
  const nameOf = (phone: string, jid: string) => {
    if (phone && phone === r.ownPhone) return 'Anda'
    const m = metaOf.members.find((x) => (phone && x.phone === phone) || x.jid === jid)
    return m?.name || fmtPhone(phone || digits(jid))
  }
  const who = fromMe ? 'Anda' : (metaOf.members.find((x) => actor && x.phone === actor)?.name || msg.pushName || fmtPhone(actor))
  const people = ['add', 'join', 'remove', 'leave', 'promote', 'demote'].includes(kind) ? stubParticipants(params) : []
  const text = groupStubLine(kind, who, people.map((p) => nameOf(p.phone, p.jid)), params)
  if (!text) return null
  const ts = typeof msg.messageTimestamp === 'number' ? msg.messageTimestamp : Number(msg.messageTimestamp ?? 0)
  return {
    wamid: key.id,
    session,
    account: r.ownPhone,
    from: fromMe ? r.ownPhone : actor,
    to: fromMe ? '' : r.ownPhone,
    chat_id: chat,
    is_group: true,
    group_meta: metaOf,
    sender_name: '', // the line names who did it; the chat list must not prefix it with a name
    text,
    media_meta: { kind: 'system', mime: '', file_name: '', size: 0, system: kind },
    timestamp: new Date((ts || Date.now() / 1000) * 1000).toISOString(),
    from_me: fromMe,
    is_history: false,
    transport: 'bridge',
  }
}

/** Maps a Baileys message to a WaEvent (a reaction too: react_to), or null for protocol messages and status updates. */
export async function toWaEvent(session: string, msg: WAMessage, r: Resolver, history: boolean): Promise<WaEvent | null> {
  const key = msg.key
  if (!key?.id || !isConversation(key.remoteJid)) return null
  if (!msg.message) {
    // a group's own line (created, renamed, a member added or gone) — live only; a synced history has them by the
    // hundred and the group is known by then
    if (!history && isJidGroup(key.remoteJid) && msg.messageStubType) return groupStub(session, msg, r)
    return null
  }
  const edit = editOf(msg.message)
  const revoke = revokeOf(msg.message)
  const reaction = reactionOf(msg.message)
  const { text, media, quoted } = edit ? { text: edit.text, media: undefined, quoted: undefined }
    : revoke ? { text: '', media: undefined, quoted: undefined }
    : reaction ? { text: reaction.emoji, media: undefined, quoted: undefined }
    : messageText(msg.message)
  if (!text && !media && !revoke && !reaction) return null
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
    account: r.ownPhone,
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
  if (media) {
    const ref = mediaRef(msg)
    ev.media_meta = ref ? { ...media, ref } : media
  }
  if (edit) ev.edit_of = edit.id
  if (revoke) ev.revoke_of = revoke
  if (reaction) ev.react_to = reaction.id
  if (quoted) ev.quoted = quoted
  if (group) ev.group_meta = (await r.groupMeta(chat)) ?? { name: '', members: [] }
  const tagged = edit || revoke || reaction ? [] : mentionedJids(msg.message)
  if (tagged.length) {
    const members = ev.group_meta?.members ?? []
    ev.mentions = await Promise.all(tagged.map(async (jid) => {
      const member = members.find((x) => x.jid === jid || (x.phone && x.phone === digits(jid)))
      const phone = member?.phone || (isLidUser(jid) ? digits((await r.pnForLid(jid)) ?? '') : digits(jid))
      return { token: digits(jid), phone, name: member?.name ?? '' }
    }))
  }
  return ev
}

/** Group metadata in the API's shape; participants are resolved to phone numbers when known and admins marked. A
 * member's name: the name saved on the phone (nameOf: saved, verified business, then the profile name the phone knows),
 * else the name WhatsApp gives with the group (their profile name). */
export function toGroupMeta(subject: string, participants: { id: string; phoneNumber?: string; name?: string; notify?: string; admin?: string | null }[],
  nameOf?: (jid: string, phone: string) => string): GroupMeta {
  return {
    name: subject,
    members: participants.map(p => {
      const phone = digits(p.phoneNumber ?? (isLidUser(p.id) ? '' : p.id))
      const m: GroupMember = { jid: p.id, phone, name: nameOf?.(p.id, phone) || p.name || p.notify || '' }
      if (p.admin) m.admin = true
      return m
    }),
  }
}
