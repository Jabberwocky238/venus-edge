import type { ReactNode } from 'react'

type BoardShellProps = {
  title: string
  resource: string
  description: string
  hostname: string
  onHostnameChange: (value: string) => void
  onLoad: () => void
  onFlush: () => void
  loading: boolean
  flushing: boolean
  dirty: boolean
  message: string | null
  error: string | null
  sidebarExtra?: ReactNode
  children: ReactNode
}

export default function BoardShell({
  title,
  resource,
  description,
  hostname,
  onHostnameChange,
  onLoad,
  onFlush,
  loading,
  flushing,
  dirty,
  message,
  error,
  sidebarExtra,
  children,
}: BoardShellProps) {
  return (
    <section className="grid gap-6 xl:grid-cols-[22rem_minmax(0,1fr)]">
      <aside className="rounded-3xl border border-slate-200/80 bg-white/90 p-6 shadow-sm">
        <div className="flex items-start justify-between gap-4">
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.24em] text-cyan-700">
              Resource
            </p>
            <h2 className="mt-2 text-2xl font-semibold tracking-tight text-slate-950">
              {title}
            </h2>
          </div>
          <span className="rounded-full bg-slate-950 px-3 py-1.5 text-xs font-semibold uppercase tracking-[0.16em] text-white">
            {resource}
          </span>
        </div>

        <p className="mt-4 text-sm leading-7 text-slate-600">{description}</p>

        <label className="mt-6 block">
          <span className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">
            Hostname
          </span>
          <input
            value={hostname}
            onChange={(event) => onHostnameChange(event.target.value)}
            placeholder="example.com"
            className="mt-2 w-full rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 font-mono text-sm text-slate-900 outline-none transition focus:border-cyan-500 focus:bg-white"
          />
        </label>

        <div className="mt-4 flex flex-wrap gap-3">
          <button
            type="button"
            onClick={onLoad}
            disabled={loading || flushing}
            className="rounded-full bg-cyan-600 px-4 py-2 text-sm font-semibold text-white transition hover:bg-cyan-700 disabled:cursor-not-allowed disabled:bg-cyan-300"
          >
            {loading ? 'Loading...' : 'Load'}
          </button>
          <button
            type="button"
            onClick={onFlush}
            disabled={!dirty || loading || flushing}
            className={`rounded-full px-4 py-2 text-sm font-semibold text-white transition ${
              dirty
                ? 'bg-emerald-600 hover:bg-emerald-700'
                : 'bg-slate-300'
            } disabled:cursor-not-allowed`}
          >
            {flushing ? 'Writing...' : 'Flush'}
          </button>
        </div>

        {sidebarExtra ? <div className="mt-6">{sidebarExtra}</div> : null}

        {message ? (
          <div className="mt-6 rounded-2xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-800">
            {message}
          </div>
        ) : null}

        {error ? (
          <div className="mt-4 rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-800">
            {error}
          </div>
        ) : null}
      </aside>

      <div className="min-w-0">{children}</div>
    </section>
  )
}
