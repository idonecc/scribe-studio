// SPDX-License-Identifier: GPL-3.0-or-later
import { useEffect, useMemo, useState } from 'react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Download as DownloadIcon,
  Film,
  FolderOpen,
  FileText,
  Plus,
  X,
  RotateCw,
  Trash2,
} from 'lucide-react'
import {
  ListTasks,
  OpenInFinder,
  ListTranscripts,
  RetryTranscribe,
  ListExternalTasks,
  RetryExternal,
  CancelExternal,
  RemoveExternal,
} from '../../wailsjs/go/scribe/App'
import type { external, pipeline, sphkit } from '../../wailsjs/go/models'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { TranscribeProgress } from '@/components/TranscribeProgress'
import { AddURLDialog } from '@/components/AddURLDialog'
import { toast } from 'sonner'

type SphTask = sphkit.TaskSummary
type ExtTask = external.Task
type Job = pipeline.Job

// UnifiedRow normalises both wx_channel and external tasks into a
// single shape the row renderer can consume. We keep the original
// object on `.raw` so retry/cancel handlers can dispatch correctly.
type UnifiedRow = {
  id: string
  kind: 'wx_channel' | 'external'
  title: string
  spec: string // resolution / quality label
  size: number
  downloaded: number
  speed: number
  status: string
  path: string
  filename: string
  createdAt: string
  updatedAt: string
  progressMsg?: string
  errorMsg?: string
  raw: SphTask | ExtTask
}

export function DownloadsPage() {
  const [wxTasks, setWxTasks] = useState<SphTask[]>([])
  const [extTasks, setExtTasks] = useState<ExtTask[]>([])
  const [filter, setFilter] = useState<'all' | 'running' | 'done' | 'error'>('all')
  const [loading, setLoading] = useState(true)
  const [transcripts, setTranscripts] = useState<Record<string, Job>>({})
  const [showAddDialog, setShowAddDialog] = useState(false)

  // Poll wx_channel + external in parallel every 2s. External tasks
  // also push live updates via the "external:task" event so the bar
  // doesn't visibly tick — the poll just keeps us in sync if we miss
  // an event.
  useEffect(() => {
    let cancelled = false
    async function pull() {
      try {
        const [r, ext] = await Promise.all([
          ListTasks(filter, 1, 50).catch(() => ({ tasks: [], total: 0 } as any)),
          ListExternalTasks().catch(() => [] as ExtTask[]),
        ])
        if (cancelled) return
        setWxTasks(r.tasks ?? [])
        setExtTasks(ext ?? [])
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    pull()
    const id = setInterval(pull, 2000)
    return () => {
      cancelled = true
      clearInterval(id)
    }
  }, [filter])

  // Live updates from the external manager.
  useEffect(() => {
    const offTask = EventsOn('external:task', (t: ExtTask) => {
      setExtTasks((prev) => {
        const idx = prev.findIndex((x) => x.id === t.id)
        if (idx === -1) return [t, ...prev]
        const next = prev.slice()
        next[idx] = t
        return next
      })
    })
    const offRemove = EventsOn('external:remove', (id: string) => {
      setExtTasks((prev) => prev.filter((t) => t.id !== id))
    })
    return () => {
      offTask()
      offRemove()
    }
  }, [])

  // Transcript jobs keyed by taskID — shared between wx_channel and
  // external rows so both can show the transcribe-progress pill.
  useEffect(() => {
    let cancelled = false
    ListTranscripts()
      .then((jobs) => {
        if (cancelled) return
        const map: Record<string, Job> = {}
        ;(jobs ?? []).forEach((j) => (map[j.taskID] = j))
        setTranscripts(map)
      })
      .catch(() => {})

    const off = EventsOn('transcribe:job', (j: Job) => {
      setTranscripts((prev) => ({ ...prev, [j.taskID]: j }))
    })
    return () => {
      cancelled = true
      off()
    }
  }, [])

  const filters: { key: typeof filter; label: string }[] = [
    { key: 'all', label: '全部' },
    { key: 'running', label: '进行中' },
    { key: 'done', label: '已完成' },
    { key: 'error', label: '失败' },
  ]

  // Merge + filter + sort: we apply the same status-bucket logic
  // to both sources so the filter chip behaves consistently.
  const rows = useMemo<UnifiedRow[]>(() => {
    const wxRows: UnifiedRow[] = wxTasks.map((t) => ({
      id: t.id,
      kind: 'wx_channel' as const,
      title: t.title || t.filename || t.id,
      spec: t.spec || '视频号',
      size: t.size,
      downloaded: t.downloaded,
      speed: t.speed,
      status: t.status,
      path: t.path,
      filename: t.filename,
      createdAt: t.createdAt,
      updatedAt: t.updatedAt,
      raw: t,
    }))
    const extRows: UnifiedRow[] = extTasks.map((t) => ({
      id: t.id,
      kind: 'external' as const,
      title: t.title || t.url,
      spec: t.formatHint || t.site || '链接',
      size: t.totalBytes > 0 ? t.totalBytes : 0,
      downloaded: t.downloaded,
      speed: t.speed,
      status: t.status,
      path: t.path,
      filename: t.filename,
      createdAt: t.createdAt,
      updatedAt: t.updatedAt,
      progressMsg: t.progressMsg,
      errorMsg: t.error,
      raw: t,
    }))
    const all = wxRows.concat(extRows)
    const matchesFilter = (r: UnifiedRow): boolean => {
      switch (filter) {
        case 'all':
          return true
        case 'running':
          return r.status === 'running' || r.status === 'downloading' || r.status === 'pending' || r.status === 'probing' || r.status === 'merging'
        case 'done':
          return r.status === 'done' || r.status === 'completed'
        case 'error':
          return r.status === 'error' || r.status === 'failed' || r.status === 'canceled'
      }
    }
    return all
      .filter(matchesFilter)
      .sort((a, b) => (b.createdAt > a.createdAt ? 1 : -1))
  }, [wxTasks, extTasks, filter])

  return (
    <>
      <Card>
        <CardHeader className="flex-row items-start justify-between gap-2 space-y-0">
          <div>
            <CardTitle>下载记录</CardTitle>
            <CardDescription>
              共 {rows.length} 条 · 数据来源 微信视频号 + yt-dlp
            </CardDescription>
          </div>
          <div className="flex items-center gap-2">
            <div className="flex items-center gap-1 rounded-md border border-border/60 bg-muted/40 p-0.5">
              {filters.map((f) => (
                <button
                  key={f.key}
                  onClick={() => setFilter(f.key)}
                  className={
                    'rounded-[5px] px-2.5 py-0.5 text-[11px] font-medium transition-colors ' +
                    (filter === f.key
                      ? 'bg-background text-foreground shadow-sm ring-1 ring-border/70'
                      : 'text-muted-foreground hover:text-foreground')
                  }
                >
                  {f.label}
                </button>
              ))}
            </div>
            <Button
              size="sm"
              variant="default"
              className="h-7 gap-1 px-2.5"
              onClick={() => setShowAddDialog(true)}
            >
              <Plus className="h-3.5 w-3.5" /> 添加链接
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {loading ? (
            <EmptyState icon={<DownloadIcon className="h-10 w-10 opacity-40" />} text="读取中…" />
          ) : rows.length === 0 ? (
            <EmptyState
              icon={<DownloadIcon className="h-10 w-10 opacity-40" />}
              text="暂无下载记录"
              sub="点击右上角「+ 添加链接」粘贴 YouTube / B站 等 URL，或启动代理后在视频号里点注入的下载按钮。"
            />
          ) : (
            <div className="divide-y divide-border/40">
              {rows.map((r) => (
                <UnifiedTaskRow key={r.id} row={r} transcript={transcripts[r.id]} />
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <AddURLDialog
        open={showAddDialog}
        onClose={() => setShowAddDialog(false)}
      />
    </>
  )
}

function EmptyState({
  icon,
  text,
  sub,
}: {
  icon: React.ReactNode
  text: string
  sub?: string
}) {
  return (
    <div className="flex flex-col items-center justify-center gap-3 py-16 text-center text-muted-foreground">
      {icon}
      <div>
        <p className="text-sm">{text}</p>
        {sub && <p className="mx-auto mt-1 max-w-sm text-xs opacity-70">{sub}</p>}
      </div>
    </div>
  )
}

function UnifiedTaskRow({ row, transcript }: { row: UnifiedRow; transcript?: Job }) {
  const percent =
    row.size > 0 ? Math.min(100, Math.round((row.downloaded / row.size) * 100)) : 0

  const displayTitle = useMemo(() => {
    const raw = row.title
    // wx_channel titles often have trailing #tags; strip for readability.
    const idx = raw.indexOf('#')
    return idx > 0 ? raw.slice(0, idx).trim() : raw
  }, [row.title])

  async function reveal() {
    if (!row.path || !row.filename) return
    await OpenInFinder(`${row.path}/${row.filename}`)
  }

  async function triggerTranscribe() {
    try {
      // RetryTranscribe is shared between wx_channel and external IDs;
      // the pipeline routes by Job.Source under the hood.
      await RetryTranscribe(row.id)
    } catch {
      /* surfaced via toast in TranscriptsPage; silent here */
    }
  }

  async function retryDownload() {
    try {
      if (row.kind === 'external') {
        await RetryExternal(row.id)
        toast.success('已重新开始下载')
      }
    } catch (err) {
      toast.error(String(err).replace(/^Error: /, ''))
    }
  }

  async function cancelDownload() {
    try {
      await CancelExternal(row.id)
    } catch (err) {
      toast.error(String(err).replace(/^Error: /, ''))
    }
  }

  async function removeRow() {
    try {
      await RemoveExternal(row.id)
    } catch (err) {
      toast.error(String(err).replace(/^Error: /, ''))
    }
  }

  const isRunning =
    row.status === 'running' ||
    row.status === 'downloading' ||
    row.status === 'pending' ||
    row.status === 'probing' ||
    row.status === 'merging'
  const isDone = row.status === 'done' || row.status === 'completed'
  const isError = row.status === 'error' || row.status === 'failed' || row.status === 'canceled'

  return (
    <div className="flex items-center gap-4 py-3">
      <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-muted/60 text-muted-foreground">
        <Film className="h-5 w-5" />
      </div>

      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <div className="truncate text-sm font-medium" title={row.title}>
            {displayTitle}
          </div>
          {row.spec && (
            <Badge
              variant="outline"
              className="h-5 shrink-0 px-1.5 py-0 font-mono text-[10px]"
              title={row.kind === 'external' ? '通过 yt-dlp 下载' : '通过视频号代理下载'}
            >
              {row.spec}
            </Badge>
          )}
        </div>
        <div className="mt-1 flex items-center gap-3 text-xs text-muted-foreground">
          <span>{humanSize(row.size > 0 ? row.size : row.downloaded)}</span>
          {isRunning && row.speed > 0 && (
            <>
              <span>·</span>
              <span>{humanSize(row.speed)}/s</span>
            </>
          )}
          {isRunning && row.size > 0 && (
            <>
              <span>·</span>
              <span>{percent}%</span>
            </>
          )}
          {row.progressMsg && (
            <>
              <span>·</span>
              <span>{row.progressMsg}</span>
            </>
          )}
          <span>·</span>
          <span className="font-mono">{shortTime(row.updatedAt)}</span>
        </div>
        {isRunning && (
          <div className="mt-2 h-1 w-full overflow-hidden rounded-full bg-muted">
            <div
              className="h-full rounded-full bg-emerald-500 transition-all"
              style={{ width: `${percent}%` }}
            />
          </div>
        )}
        {isError && row.errorMsg && (
          <div className="mt-1 truncate text-[11px] text-destructive" title={row.errorMsg}>
            {row.errorMsg}
          </div>
        )}
      </div>

      <div className="flex shrink-0 items-center gap-2">
        <StatusBadge status={row.status} />
        {isDone && (
          transcript ? (
            <TranscribeProgress job={transcript} />
          ) : (
            <Button
              variant="outline"
              size="sm"
              className="h-7 gap-1 px-2"
              onClick={triggerTranscribe}
              title="转写这条"
            >
              <FileText className="h-3.5 w-3.5" /> 转写
            </Button>
          )
        )}
        {isDone && (
          <Button variant="ghost" size="sm" className="h-7 px-2" onClick={reveal} title="在 Finder 中显示">
            <FolderOpen className="h-3.5 w-3.5" />
          </Button>
        )}
        {row.kind === 'external' && isRunning && (
          <Button variant="ghost" size="sm" className="h-7 px-2" onClick={cancelDownload} title="取消下载">
            <X className="h-3.5 w-3.5" />
          </Button>
        )}
        {row.kind === 'external' && isError && (
          <Button variant="ghost" size="sm" className="h-7 px-2" onClick={retryDownload} title="重试下载">
            <RotateCw className="h-3.5 w-3.5" />
          </Button>
        )}
        {row.kind === 'external' && (isError || isDone) && (
          <Button variant="ghost" size="sm" className="h-7 px-2" onClick={removeRow} title="从列表移除">
            <Trash2 className="h-3.5 w-3.5" />
          </Button>
        )}
      </div>
    </div>
  )
}

function StatusBadge({ status }: { status: string }) {
  if (status === 'done' || status === 'completed')
    return <Badge variant="success">完成</Badge>
  if (status === 'running' || status === 'downloading')
    return <Badge variant="warning">下载中</Badge>
  if (status === 'merging') return <Badge variant="warning">合并中</Badge>
  if (status === 'pending' || status === 'probing')
    return <Badge variant="outline">排队中</Badge>
  if (status === 'error' || status === 'failed')
    return <Badge variant="destructive">失败</Badge>
  if (status === 'canceled') return <Badge variant="outline">已取消</Badge>
  if (status === 'paused') return <Badge variant="outline">已暂停</Badge>
  return <Badge variant="outline">{status}</Badge>
}

function humanSize(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let n = bytes
  let i = 0
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024
    i++
  }
  return `${n.toFixed(n >= 10 || i === 0 ? 0 : 1)} ${units[i]}`
}

function shortTime(iso: string): string {
  if (!iso) return ''
  try {
    const d = new Date(iso)
    const hh = String(d.getHours()).padStart(2, '0')
    const mm = String(d.getMinutes()).padStart(2, '0')
    return `${d.getMonth() + 1}/${d.getDate()} ${hh}:${mm}`
  } catch {
    return iso.slice(0, 16)
  }
}
