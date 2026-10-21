import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, type Items } from '../../api/client'
import type { BoardItem, CreditOverview, KPI, ThreadView } from '../../api/types'
import { useOrch } from '../../app/orch'
import { STAGES } from '../../app/cycle'
import { useBrief, useCycleLatest, useQueue, useStockCritical, useStockPush, useTrend, useWAStatus } from '../../app/queries'
import type { CycleProgress } from '../control/progress'
import { boardFacts, crewLines, crewStatus, waitingBy, CREW_BY_AGENT, type CrewFacts, type CrewKey } from './station'

const ENDED = ['done', 'partial', 'failed']

/** The ten astronauts' data, shared by the Stasiun Orbit scene and the "Hasil overview AI" card: the counts each
 * one looks at, what each is doing now (the cycle being followed, else idle) and the last finished cycle. */
export function useCrew(board: BoardItem[], kpi: KPI | undefined, can: (screen: string) => boolean) {
  const { data: push } = useStockPush()
  const { data: critical } = useStockCritical()
  // new numbers that wrote in first, as a count only — the scene and the card never show who
  const { data: newNumbers } = useQuery({
    queryKey: ['station', 'new-numbers'],
    enabled: can('chat'),
    staleTime: 60_000,
    refetchInterval: 60_000,
    queryFn: () => api.get<Items<ThreadView>>('/chat/threads?tab=new').then((r) => r.items.filter((t) => t.kind === 'new' && t.contact_type !== 'supplier' && t.last_direction !== 'out').length),
  })
  const { data: credit } = useQuery({ queryKey: ['credit', 'overview'], enabled: can('ar'), queryFn: () => api.get<CreditOverview>('/credit/overview') })
  const { data: wa } = useWAStatus()
  const { data: brief } = useBrief()
  const monthly = useTrend('months')
  const { data: queue = [] } = useQueue()
  const waiting = queue.filter((q) => q.status === 'proposed')

  const { s: live } = useOrch()
  const { data: latest } = useCycleLatest()
  const running = live.running || !!latest?.running
  const stage = live.running ? Math.min(live.shown, STAGES.length - 1) : Math.max(0, STAGES.indexOf(latest?.cycle?.stage ?? 'ingest'))
  const lastDone = latest?.last_done ?? null
  // the cycle the astronauts act out: any cycle that runs while the dashboard is open, kept after it ends until closed
  const [follow, setFollow] = useState<{ id: string; mine: boolean } | null>(null)
  const [seen, setSeen] = useState<string | null>(null)
  const runningId = live.running && live.cycleId ? live.cycleId : latest?.running ? latest.cycle?.id ?? null : null
  if (runningId && runningId !== seen) { // a new cycle started: follow it (state adjusted while rendering)
    setSeen(runningId)
    setFollow({ id: runningId, mine: live.mine })
  }
  const { data: prog } = useQuery({
    queryKey: ['cycle-progress', follow?.id],
    enabled: !!follow,
    queryFn: () => api.get<CycleProgress>(`/cycles/${follow!.id}/progress`),
    refetchInterval: (q) => (ENDED.includes(q.state.data?.status ?? '') ? false : 2000),
  })
  const ended = !!prog && prog.cycle_id === follow?.id && ENDED.includes(prog.status)
  // a scheduled cycle's result closes by itself; one started here stays until it is closed
  useEffect(() => {
    if (!ended || !follow || follow.mine) return
    const t = setTimeout(() => setFollow(null), 45_000)
    return () => clearTimeout(t)
  }, [ended, follow])
  // the last finished cycle, for the card when no cycle is followed
  const { data: lastProg } = useQuery({
    queryKey: ['cycle-progress', lastDone?.id],
    enabled: !follow && !!lastDone,
    staleTime: 60_000,
    queryFn: () => api.get<CycleProgress>(`/cycles/${lastDone!.id}/progress`),
  })

  const startedAt = prog?.stages.find((x) => x.stage === 'ingest')?.started_at ?? null
  const fresh = startedAt ? waiting.filter((q) => Date.parse(q.created_at) >= Date.parse(startedAt)).length : 0

  const om = monthly.data?.series.omzet ?? []
  const lastIdx = om.reduce<number>((acc, v, i) => (v != null ? i : acc), -1)
  const facts: CrewFacts = {
    ...boardFacts(board),
    omzetMonth: lastIdx >= 0 ? om[lastIdx] : undefined,
    omzetPrev: lastIdx >= 1 ? om[lastIdx - 1] : undefined,
    so: brief?.counts.so,
    payments: brief?.counts.payments,
    waMessages: can('chat') ? brief?.counts.wa : undefined,
    unread: can('chat') ? wa?.items.reduce((a, n) => a + (n.unread_count ?? 0), 0) : undefined,
    newNumbers,
    tight: kpi?.tight_count ?? boardFacts(board).tight,
    overdue: credit?.overdue,
    overdueDealers: credit?.overdue_dealers,
    aging: push?.length,
    agingValue: push?.reduce((a, x) => a + x.value, 0),
    critical: critical?.length,
    waiting: waiting.length,
    waitingBy: waitingBy(waiting.map((q) => q.agent)),
  }
  const lines = crewLines(facts)
  const following = !!follow && !!prog && prog.cycle_id === follow.id
  const active = following || running
  const failed = ended && prog?.status === 'failed'
  const progStage = following && prog?.stage ? Math.max(0, STAGES.indexOf(prog.stage as (typeof STAGES)[number])) : stage
  const status = crewStatus(lines, active ? { active, stage: ended ? STAGES.length : progStage, ended, failed, agents: following ? prog!.agents : [], fresh } : null)
  // what the last finished cycle said per astronaut ("3 saran"), for the card
  const lastSaid = useMemo(() => {
    const out: Partial<Record<CrewKey, string>> = {}
    for (const a of lastProg?.agents ?? []) {
      const k = CREW_BY_AGENT[a.agent]
      if (k && a.summary) out[k] = a.state === 'failed' ? 'gagal' : a.summary
    }
    return out
  }, [lastProg])

  return { facts, lines, status, running, stage, lastDone, latest, follow, setFollow, prog: following ? prog : undefined, ended, fresh, waiting, lastProg, lastSaid, push, critical }
}

export type Crew = ReturnType<typeof useCrew>
