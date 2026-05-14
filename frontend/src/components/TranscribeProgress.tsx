// SPDX-License-Identifier: GPL-3.0-or-later
import { Badge } from '@/components/ui/badge'
import type { pipeline } from '../../wailsjs/go/models'

type Stage = pipeline.Job['stage']

const labels: Record<Stage | string, string> = {
  pending: '排队中',
  extracting: '提取音频',
  transcribing: '转写中',
  saving: '保存中',
  done: '已转写',
  failed: '失败',
}

export function TranscribeProgress({ job }: { job?: pipeline.Job | null }) {
  if (!job) return <Badge variant="outline" className="opacity-50">未转写</Badge>

  const stage = job.stage
  const label = labels[stage] ?? stage

  if (stage === 'done') return <Badge variant="success">已转写</Badge>
  if (stage === 'failed') return <Badge variant="destructive" title={job.error}>失败</Badge>
  if (stage === 'pending') return <Badge variant="outline">{label}</Badge>

  // extracting / transcribing / saving — show % if known, else just label
  const frac = job.progress
  const pct = frac >= 0 && frac <= 1 ? Math.round(frac * 100) : null
  return (
    <div className="flex items-center gap-2">
      <Badge variant="warning">{label}</Badge>
      {pct !== null ? (
        <span className="text-[11px] tabular-nums text-muted-foreground">{pct}%</span>
      ) : job.progressMsg ? (
        <span className="text-[11px] text-muted-foreground">{job.progressMsg}</span>
      ) : null}
    </div>
  )
}
