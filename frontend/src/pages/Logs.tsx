// SPDX-License-Identifier: GPL-3.0-or-later
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ScrollText, Pause, Play, Trash2, Copy, ArrowDownToLine } from 'lucide-react'
import { ListLogs, ClearLogs } from '../../wailsjs/go/scribe/App'
import type { logbus } from '../../wailsjs/go/models'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { cn } from '@/lib/utils'
import { toast } from 'sonner'

type LogEntry = logbus.Entry
type Level = LogEntry['level']

const LEVELS: { key: Level | 'all'; label: string }[] = [
  { key: 'all', label: '全部' },
  { key: 'info', label: 'info' },
  { key: 'warn', label: 'warn' },
  { key: 'error', label: 'error' },
  { key: 'debug', label: 'debug' },
]

// MAX_ROWS caps the in-memory tail so a runaway log producer can't
// blow up the renderer. 1500 covers any practical user session.
const MAX_ROWS = 1500

export function LogsPage() {
  const [entries, setEntries] = useState<LogEntry[]>([])
  const [filter, setFilter] = useState<Level | 'all'>('all')
  const [keyword, setKeyword] = useState('')
  const [paused, setPaused] = useState(false)
  const [autoScroll, setAutoScroll] = useState(true)
  const scrollRef = useRef<HTMLDivElement | null>(null)
  // EventsOn captures `paused` in its closure on the first render.
  // Mirror it through a ref so the listener can read the live value
  // without us having to re-subscribe (which would race against
  // backend-emitted events and lose entries between subscriptions).
  const pausedRef = useRef(false)
  useEffect(() => {
    pausedRef.current = paused
  }, [paused])

  // Initial fetch + live tail.
  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const initial = await ListLogs(MAX_ROWS)
        if (!cancelled) setEntries(initial ?? [])
      } catch (err) {
        if (!cancelled) toast.error(String(err).replace(/^Error: /, ''))
      }
    })()

    const off = EventsOn('log:entry', (e: LogEntry) => {
      // While paused the visible buffer is frozen — the user is
      // reading a specific moment. We still record the entry into
      // the rolling buffer (so unpausing surfaces the catch-up tail
      // up to MAX_ROWS), we just don't push it through React state
      // until they resume.
      if (pausedRef.current) return
      setEntries((prev) => {
        const next = prev.concat(e)
        if (next.length > MAX_ROWS) next.splice(0, next.length - MAX_ROWS)
        return next
      })
    })
    return () => {
      cancelled = true
      off()
    }
  }, [])

  // When the user un-pauses, fetch the latest tail so they see what
  // they missed. We deliberately re-pull from the backend rather than
  // trying to merge a JS-side queue — the backend's ring buffer is
  // the source of truth and de-dupes automatically by entry order.
  useEffect(() => {
    if (paused) return
    let cancelled = false
    ListLogs(MAX_ROWS)
      .then((latest) => {
        if (cancelled) return
        setEntries(latest ?? [])
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [paused])

  // Auto-scroll to bottom when new entries arrive, unless the user
  // has explicitly paused or scrolled up to inspect history.
  useEffect(() => {
    if (paused || !autoScroll) return
    const el = scrollRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [entries, paused, autoScroll])

  // Detect manual scroll-up so we can suspend auto-scroll until the
  // user returns to the bottom. This matches the behavior most
  // terminal-like UIs use; otherwise reading older entries is
  // impossible because every new emit yanks the view back down.
  const onScroll = useCallback((e: React.UIEvent<HTMLDivElement>) => {
    const el = e.currentTarget
    const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 24
    setAutoScroll(nearBottom)
  }, [])

  const filtered = useMemo(() => {
    const kw = keyword.trim().toLowerCase()
    return entries.filter((e) => {
      if (filter !== 'all' && e.level !== filter) return false
      if (!kw) return true
      return (
        e.message.toLowerCase().includes(kw) ||
        e.source.toLowerCase().includes(kw)
      )
    })
  }, [entries, filter, keyword])

  const plainText = useMemo(
    () =>
      filtered
        .map(
          (e) =>
            `${formatTs(e.timestamp)} [${e.level.toUpperCase()}] ${e.source}: ${e.message}`
        )
        .join('\n'),
    [filtered]
  )

  async function copyAll() {
    if (!plainText) {
      toast.message('当前视图没有日志')
      return
    }
    try {
      await navigator.clipboard.writeText(plainText)
      toast.success(`已复制 ${filtered.length} 行`)
    } catch {
      toast.error('复制失败')
    }
  }

  async function clear() {
    try {
      await ClearLogs()
      setEntries([])
      toast.success('已清空')
    } catch (err) {
      toast.error(String(err).replace(/^Error: /, ''))
    }
  }

  function scrollToBottom() {
    const el = scrollRef.current
    if (el) {
      el.scrollTop = el.scrollHeight
      setAutoScroll(true)
    }
  }

  return (
    <Card className="flex h-full max-h-[calc(100vh-110px)] flex-col">
      <CardHeader className="flex-row items-start justify-between gap-2 space-y-0 pb-3">
        <div>
          <CardTitle>日志</CardTitle>
          <CardDescription>
            共 {filtered.length} 条 · 实时流自核心服务（点击「暂停」可冻结视图）
          </CardDescription>
        </div>
        <div className="flex items-center gap-2">
          <input
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
            placeholder="搜索…"
            className={cn(
              'h-7 w-44 rounded-md border border-border/60 bg-background px-2 text-xs',
              'focus:border-foreground/30 focus:outline-none'
            )}
          />
          <div className="flex items-center gap-1 rounded-md border border-border/60 bg-muted/40 p-0.5">
            {LEVELS.map((l) => (
              <button
                key={l.key}
                onClick={() => setFilter(l.key)}
                className={
                  'rounded-[5px] px-2 py-0.5 text-[11px] font-medium transition-colors ' +
                  (filter === l.key
                    ? 'bg-background text-foreground shadow-sm ring-1 ring-border/70'
                    : 'text-muted-foreground hover:text-foreground')
                }
              >
                {l.label}
              </button>
            ))}
          </div>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 gap-1 px-2"
            onClick={() => setPaused((p) => !p)}
            title={paused ? '继续' : '暂停（停止接收新日志）'}
          >
            {paused ? <Play className="h-3.5 w-3.5" /> : <Pause className="h-3.5 w-3.5" />}
            {paused ? '继续' : '暂停'}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 gap-1 px-2"
            onClick={copyAll}
            title="复制当前视图所有行"
          >
            <Copy className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 gap-1 px-2"
            onClick={clear}
            title="清空（仅清 UI 缓冲，磁盘日志不受影响）"
          >
            <Trash2 className="h-3.5 w-3.5" />
          </Button>
        </div>
      </CardHeader>
      <CardContent className="relative flex-1 overflow-hidden pb-3 pt-0">
        {filtered.length === 0 ? (
          <div className="flex h-full flex-col items-center justify-center gap-3 text-center text-muted-foreground">
            <ScrollText className="h-10 w-10 opacity-40" />
            <div>
              <p className="text-sm">暂无日志</p>
              <p className="mx-auto mt-1 max-w-sm text-xs opacity-70">
                启动代理、添加下载、或开始转写后，关键事件会实时出现在这里。
              </p>
            </div>
          </div>
        ) : (
          <div
            ref={scrollRef}
            onScroll={onScroll}
            className={cn(
              'h-full overflow-y-auto rounded-md border border-border/40 bg-muted/30 font-mono text-[11.5px] leading-relaxed',
              'p-3'
            )}
          >
            {filtered.map((e, i) => (
              <LogRow key={i} entry={e} />
            ))}
          </div>
        )}

        {!autoScroll && filtered.length > 0 && (
          <Button
            size="sm"
            variant="default"
            className="absolute bottom-5 right-5 h-7 gap-1 px-2 shadow-md"
            onClick={scrollToBottom}
            title="跳到最新"
          >
            <ArrowDownToLine className="h-3.5 w-3.5" /> 最新
          </Button>
        )}
      </CardContent>
    </Card>
  )
}

function LogRow({ entry }: { entry: LogEntry }) {
  return (
    <div className="flex items-start gap-2 py-px">
      <span className="shrink-0 text-muted-foreground/70">{formatTs(entry.timestamp)}</span>
      <LevelChip level={entry.level} />
      <span className="shrink-0 text-muted-foreground">{entry.source}</span>
      <span className="break-all">{entry.message}</span>
    </div>
  )
}

function LevelChip({ level }: { level: Level }) {
  const cls: Record<Level, string> = {
    debug: 'text-muted-foreground',
    info: 'text-sky-600 dark:text-sky-400',
    warn: 'text-amber-600 dark:text-amber-400',
    error: 'text-red-600 dark:text-red-400',
  }
  return (
    <span className={cn('w-12 shrink-0 font-semibold uppercase', cls[level] ?? cls.info)}>
      {level}
    </span>
  )
}

function formatTs(iso: string): string {
  // Show HH:MM:SS.mmm in the local timezone; we drop the date because
  // sessions rarely span >24h and the column would otherwise be very
  // wide in the monospace grid.
  const d = new Date(iso)
  if (isNaN(d.getTime())) return iso.slice(0, 23)
  const hh = String(d.getHours()).padStart(2, '0')
  const mm = String(d.getMinutes()).padStart(2, '0')
  const ss = String(d.getSeconds()).padStart(2, '0')
  const ms = String(d.getMilliseconds()).padStart(3, '0')
  return `${hh}:${mm}:${ss}.${ms}`
}
