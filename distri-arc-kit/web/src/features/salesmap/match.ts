// Mapping sales helpers (pure, tested): one Pengguna ↔ many BigQuery sales names, and which names look like a user.

export interface SalesProfile {
  id: string
  name: string
  branch: string
  source_system: string | null
  source_id: string | null
  active: boolean
  merged_into: string | null
  wa_number: string | null
  dealers: number
  login_email: string | null
  merged_count: number
  /** the Pengguna this name belongs to */
  user_id: string | null
  /** it is that user's main profile (not removable) */
  main: boolean
}

export interface MapUser {
  id: string
  name: string
  email: string | null
  role_name: string
  role: string
  sales_user_id: string | null
  branch: string
}

/** "Granike Monica M." → "granike": the first word, lower-case, letters only, c/k and ph/f spelled alike. */
export function nameKey(name: string): string {
  const first = name.toLocaleLowerCase('id-ID').normalize('NFKD').replace(/[^a-z\s]/g, ' ').trim().split(/\s+/)[0] ?? ''
  return first.replace(/ph/g, 'f').replace(/c/g, 'k').replace(/(.)\1+/g, '$1')
}

/** A BigQuery name that probably is this user ("Granike Monica M." for Pengguna "Granike Monika"). */
export function looksLike(userName: string, name: string): boolean {
  const k = nameKey(userName)
  return k.length >= 3 && k === nameKey(name)
}

/** The names linked to each user (main profile first, then by name). */
export function namesByUser(rows: SalesProfile[]): Map<string, SalesProfile[]> {
  const out = new Map<string, SalesProfile[]>()
  for (const r of rows) if (r.user_id) out.set(r.user_id, [...(out.get(r.user_id) ?? []), r])
  for (const list of out.values()) list.sort((a, b) => Number(b.main) - Number(a.main) || a.name.localeCompare(b.name, 'id'))
  return out
}

/** BigQuery names not linked to any user yet (a name already merged follows its main one, so it is not listed). */
export function unlinked(rows: SalesProfile[]): SalesProfile[] {
  return rows.filter((r) => !r.user_id && !r.merged_into)
}

/** Unlinked names for a user: those that look like the user first, then by dealers. */
export function candidates(rows: SalesProfile[], user: MapUser | undefined): SalesProfile[] {
  const list = unlinked(rows)
  if (!user) return list
  return [...list].sort((a, b) => Number(looksLike(user.name, b.name)) - Number(looksLike(user.name, a.name)) || b.dealers - a.dealers || a.name.localeCompare(b.name, 'id'))
}
