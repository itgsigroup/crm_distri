// Mapping sales helpers (pure, tested): which spellings from the source data probably name the same person.

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
}

/** "Granike Monica M." → "granike": the first word, lower-case, letters only, c/k and ph/f spelled alike. */
export function nameKey(name: string): string {
  const first = name.toLocaleLowerCase('id-ID').normalize('NFKD').replace(/[^a-z\s]/g, ' ').trim().split(/\s+/)[0] ?? ''
  return first.replace(/ph/g, 'f').replace(/c/g, 'k').replace(/(.)\1+/g, '$1')
}

/** Full-name key: every word, same spelling rules ("Granike Monica" ≈ "Granike Monika"). */
export function fullKey(name: string): string {
  return name.toLocaleLowerCase('id-ID').normalize('NFKD').replace(/[^a-z\s]/g, ' ').trim().split(/\s+/)
    .filter((w) => w.length > 1).map((w) => w.replace(/ph/g, 'f').replace(/c/g, 'k').replace(/(.)\1+/g, '$1')).join(' ')
}

/** For each profile not merged yet: the other unmerged profiles that look like the same person. */
export function suggestions(rows: SalesProfile[]): Map<string, SalesProfile[]> {
  const open = rows.filter((r) => !r.merged_into)
  const byKey = new Map<string, SalesProfile[]>()
  for (const r of open) {
    const k = nameKey(r.name)
    if (k.length < 3) continue
    byKey.set(k, [...(byKey.get(k) ?? []), r])
  }
  const out = new Map<string, SalesProfile[]>()
  for (const group of byKey.values()) {
    if (group.length < 2) continue
    for (const r of group) out.set(r.id, group.filter((x) => x.id !== r.id))
  }
  return out
}

/** The profile a group should be merged into: one made in GSI Orbit, else one with a login, else most dealers. */
export function bestTarget(group: SalesProfile[]): SalesProfile | undefined {
  return [...group].sort((a, b) =>
    Number(a.source_system === 'import') - Number(b.source_system === 'import') ||
    Number(!a.login_email) - Number(!b.login_email) ||
    b.dealers - a.dealers || a.name.localeCompare(b.name))[0]
}
