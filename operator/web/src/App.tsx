import { useEffect, useState } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'

type SubscriberSnapshot = {
  pod_ip: string
  agent_id: string
}

type WALStatus = {
  dir: string
  active_file: string
  index: number
  rotated_at: number
}

type ManageFileInfo = {
  path: string
  exists: boolean
  size: number
  mod_time: number
}

type OverviewResponse = {
  now_unix: number
  subscribers: SubscriberSnapshot[]
  wal: WALStatus
  wal_files: ManageFileInfo[]
}

const refreshIntervalMs = 5000

function formatTime(unix: number) {
  if (!unix) {
    return 'N/A'
  }

  return new Date(unix * 1000).toLocaleString()
}

function formatBytes(size: number) {
  if (size <= 0) {
    return '0 B'
  }

  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = size
  let unit = 0

  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }

  return `${value.toFixed(value >= 10 || unit === 0 ? 0 : 1)} ${units[unit]}`
}

function fileNameFromPath(path: string) {
  return path.split(/[/\\]/).at(-1) || path
}

async function fetchOverview(signal: AbortSignal) {
  const response = await fetch('/api/master/overview', { signal })
  if (!response.ok) {
    throw new Error(`overview request failed: ${response.status}`)
  }

  return (await response.json()) as OverviewResponse
}

function MasterManagePage() {
  const [data, setData] = useState<OverviewResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [refreshing, setRefreshing] = useState(false)

  useEffect(() => {
    let disposed = false
    let activeController: AbortController | null = null

    const load = async (silent = false) => {
      activeController?.abort()
      const controller = new AbortController()
      activeController = controller

      if (silent) {
        setRefreshing(true)
      } else {
        setLoading(true)
      }

      try {
        const next = await fetchOverview(controller.signal)
        if (!disposed) {
          setData(next)
          setError(null)
        }
      } catch (err) {
        if (!disposed && !(err instanceof DOMException && err.name === 'AbortError')) {
          setError(err instanceof Error ? err.message : 'unknown error')
        }
      } finally {
        if (!disposed && activeController === controller) {
          setLoading(false)
          setRefreshing(false)
        }
      }
    }

    void load()
    const timer = window.setInterval(() => {
      void load(true)
    }, refreshIntervalMs)

    return () => {
      disposed = true
      window.clearInterval(timer)
      activeController?.abort()
    }
  }, [])

  const subscriberCount = data?.subscribers.length ?? 0
  const walFileCount = data?.wal_files.length ?? 0
  const existingWalFiles = data?.wal_files.filter((file) => file.exists).length ?? 0

  return (
    <div className="mx-auto flex min-h-screen w-full max-w-7xl flex-col px-4 py-6 sm:px-6 lg:px-8">
      <section className="overflow-hidden rounded-[28px] border border-slate-200/80 bg-white/85 shadow-[0_24px_80px_-32px_rgba(15,23,42,0.35)] backdrop-blur">
        <div className="grid gap-8 bg-[radial-gradient(circle_at_top_left,_rgba(14,165,233,0.18),_transparent_34%),linear-gradient(135deg,_rgba(255,255,255,0.96),_rgba(248,250,252,0.92))] px-6 py-8 sm:px-8 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-start">
          <div className="space-y-4">
            <p className="text-xs font-semibold uppercase tracking-[0.35em] text-cyan-700">
              Venus Edge
            </p>
            <div className="space-y-3">
              <h1 className="text-4xl font-semibold tracking-tight text-slate-950 sm:text-5xl">
                Master Manage
              </h1>
              <p className="max-w-2xl text-sm leading-7 text-slate-600 sm:text-base">
                Inspect subscriber connections and the current WAL rotation state
                from the operator master.
              </p>
            </div>
          </div>
          <div className="flex flex-col items-start gap-3 rounded-2xl border border-slate-200/80 bg-slate-950 px-5 py-4 text-sm text-slate-200 shadow-[inset_0_1px_0_rgba(255,255,255,0.08)]">
            <span
              className={`inline-flex rounded-full px-3 py-1 text-xs font-semibold uppercase tracking-[0.2em] ${
                error
                  ? 'bg-rose-500/15 text-rose-200 ring-1 ring-inset ring-rose-400/30'
                  : 'bg-emerald-500/15 text-emerald-200 ring-1 ring-inset ring-emerald-400/30'
              }`}
            >
              {error ? 'API error' : refreshing ? 'Refreshing' : 'Healthy'}
            </span>
            <p className="text-xs uppercase tracking-[0.2em] text-slate-400">
              Snapshot
            </p>
            <p className="font-mono text-sm text-slate-100">
              {data ? formatTime(data.now_unix) : 'Loading'}
            </p>
          </div>
        </div>
      </section>

      <section className="mt-6 grid gap-4 md:grid-cols-3">
        <StatCard label="Subscribers" value={loading ? '...' : String(subscriberCount)} />
        <StatCard label="WAL Index" value={loading ? '...' : String(data?.wal.index ?? 'N/A')} />
        <StatCard
          label="Existing WAL Files"
          value={loading ? '...' : `${existingWalFiles}/${walFileCount}`}
        />
      </section>

      {error ? (
        <section className="mt-6 rounded-3xl border border-rose-200 bg-rose-50 px-6 py-5 text-rose-900">
          <h2 className="text-lg font-semibold">Request failed</h2>
          <p className="mt-2 text-sm">{error}</p>
        </section>
      ) : null}

      <section className="mt-6 grid gap-6 xl:grid-cols-[1.2fr_0.8fr]">
        <article className="rounded-3xl border border-slate-200/80 bg-white/90 p-6 shadow-sm">
          <PanelHeader kicker="Cluster" title="Subscribers" badge={String(subscriberCount)} />
          {loading && !data ? (
            <p className="mt-4 text-sm text-slate-500">Loading subscriber snapshot...</p>
          ) : subscriberCount === 0 ? (
            <p className="mt-4 text-sm text-slate-500">No active subscribers.</p>
          ) : (
            <div className="mt-5 overflow-hidden rounded-2xl border border-slate-200">
              <table className="min-w-full divide-y divide-slate-200 text-left text-sm">
                <thead className="bg-slate-50 text-slate-500">
                  <tr>
                    <th className="px-4 py-3 font-medium">Pod IP</th>
                    <th className="px-4 py-3 font-medium">Agent ID</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100 bg-white text-slate-700">
                  {data?.subscribers.map((subscriber) => (
                    <tr key={`${subscriber.pod_ip}-${subscriber.agent_id}`}>
                      <td className="px-4 py-3 font-mono text-xs sm:text-sm">
                        {subscriber.pod_ip || 'N/A'}
                      </td>
                      <td className="px-4 py-3 font-mono text-xs sm:text-sm">
                        {subscriber.agent_id || 'N/A'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </article>

        <article className="rounded-3xl border border-slate-200/80 bg-white/90 p-6 shadow-sm">
          <PanelHeader
            kicker="Storage"
            title="WAL Status"
            badge={`#${data?.wal.index ?? '-'}`}
          />
          <dl className="mt-5 space-y-4">
            <DetailRow label="Directory" value={data?.wal.dir ?? 'Loading...'} />
            <DetailRow label="Active File" value={data?.wal.active_file ?? 'Loading...'} />
            <DetailRow
              label="Rotated At"
              value={data ? formatTime(data.wal.rotated_at) : 'Loading...'}
            />
          </dl>
        </article>
      </section>

      <section className="mt-6 rounded-3xl border border-slate-200/80 bg-white/90 p-6 shadow-sm">
        <PanelHeader kicker="Disk" title="WAL Files" badge={String(walFileCount)} />
        {loading && !data ? (
          <p className="mt-4 text-sm text-slate-500">Loading WAL files...</p>
        ) : (
          <div className="mt-5 grid gap-4 md:grid-cols-2 2xl:grid-cols-3">
            {data?.wal_files.map((file) => (
              <article
                key={file.path}
                className={`rounded-2xl border p-4 transition ${
                  file.exists
                    ? 'border-emerald-200 bg-emerald-50/70'
                    : 'border-slate-200 bg-slate-50/80'
                }`}
              >
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <strong className="text-sm font-semibold text-slate-900">
                      {fileNameFromPath(file.path)}
                    </strong>
                    <p className="mt-2 break-all font-mono text-xs text-slate-500">
                      {file.path}
                    </p>
                  </div>
                  <span
                    className={`rounded-full px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.16em] ${
                      file.exists
                        ? 'bg-emerald-600 text-white'
                        : 'bg-slate-200 text-slate-700'
                    }`}
                  >
                    {file.exists ? 'Present' : 'Missing'}
                  </span>
                </div>
                <dl className="mt-5 grid gap-3 text-sm">
                  <DetailRow compact label="Size" value={formatBytes(file.size)} />
                  <DetailRow
                    compact
                    label="Updated"
                    value={file.exists ? formatTime(file.mod_time) : 'N/A'}
                  />
                </dl>
              </article>
            ))}
          </div>
        )}
      </section>
    </div>
  )
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <article className="rounded-3xl border border-slate-200/80 bg-white/85 px-5 py-5 shadow-sm backdrop-blur">
      <p className="text-xs font-semibold uppercase tracking-[0.24em] text-slate-500">
        {label}
      </p>
      <strong className="mt-3 block text-3xl font-semibold tracking-tight text-slate-950">
        {value}
      </strong>
    </article>
  )
}

function PanelHeader({
  kicker,
  title,
  badge,
}: {
  kicker: string
  title: string
  badge: string
}) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div>
        <p className="text-xs font-semibold uppercase tracking-[0.24em] text-cyan-700">
          {kicker}
        </p>
        <h2 className="mt-2 text-2xl font-semibold tracking-tight text-slate-950">
          {title}
        </h2>
      </div>
      <span className="rounded-full bg-slate-950 px-3 py-1.5 text-xs font-semibold uppercase tracking-[0.16em] text-white">
        {badge}
      </span>
    </div>
  )
}

function DetailRow({
  label,
  value,
  compact = false,
}: {
  label: string
  value: string
  compact?: boolean
}) {
  return (
    <div
      className={`rounded-2xl border border-slate-200 bg-slate-50/80 ${
        compact ? 'px-3 py-2' : 'px-4 py-3'
      }`}
    >
      <dt className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">
        {label}
      </dt>
      <dd className="mt-2 break-all font-mono text-xs text-slate-800 sm:text-sm">{value}</dd>
    </div>
  )
}

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<Navigate replace to="/master/manage" />} />
      <Route path="/master/manage" element={<MasterManagePage />} />
    </Routes>
  )
}
