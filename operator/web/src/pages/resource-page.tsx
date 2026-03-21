import { useState } from 'react'

type ResourcePageProps = {
  title: string
  resource: 'dns' | 'tls' | 'http'
  description: string
  example: string
}

type PublishResult = {
  accepted?: boolean
  message?: string
}

async function requestResource(resource: string, hostname: string) {
  const response = await fetch(
    `/api/master/${resource}?hostname=${encodeURIComponent(hostname)}`,
  )
  if (!response.ok) {
    throw new Error(await response.text())
  }
  return response.json()
}

async function publishResource(resource: string, hostname: string, payload: string) {
  const response = await fetch(
    `/api/master/${resource}?hostname=${encodeURIComponent(hostname)}`,
    {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
      },
      body: payload,
    },
  )
  if (!response.ok) {
    throw new Error(await response.text())
  }
  return (await response.json()) as PublishResult
}

export default function ResourcePage({
  title,
  resource,
  description,
  example,
}: ResourcePageProps) {
  const [hostname, setHostname] = useState('')
  const [payload, setPayload] = useState(example)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [message, setMessage] = useState<string | null>(null)

  const load = async () => {
    const trimmed = hostname.trim()
    if (!trimmed) {
      setError('hostname is required')
      return
    }

    setLoading(true)
    setError(null)
    setMessage(null)
    try {
      const data = await requestResource(resource, trimmed)
      setPayload(JSON.stringify(data, null, 2))
      setMessage(`Loaded ${resource.toUpperCase()} builder payload for ${trimmed}.`)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'load failed')
    } finally {
      setLoading(false)
    }
  }

  const save = async () => {
    const trimmed = hostname.trim()
    if (!trimmed) {
      setError('hostname is required')
      return
    }

    try {
      JSON.parse(payload)
    } catch (err) {
      setError(err instanceof Error ? `invalid json: ${err.message}` : 'invalid json')
      return
    }

    setSaving(true)
    setError(null)
    setMessage(null)
    try {
      const result = await publishResource(resource, trimmed, payload)
      setMessage(result.message || `Published ${resource.toUpperCase()} for ${trimmed}.`)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'publish failed')
    } finally {
      setSaving(false)
    }
  }

  return (
    <section className="grid gap-6 xl:grid-cols-[0.8fr_1.2fr]">
      <article className="rounded-3xl border border-slate-200/80 bg-white/90 p-6 shadow-sm">
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
            onChange={(event) => setHostname(event.target.value)}
            placeholder="example.com"
            className="mt-2 w-full rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 font-mono text-sm text-slate-900 outline-none transition focus:border-cyan-500 focus:bg-white"
          />
        </label>

        <div className="mt-4 flex flex-wrap gap-3">
          <button
            type="button"
            onClick={load}
            disabled={loading || saving}
            className="rounded-full bg-cyan-600 px-4 py-2 text-sm font-semibold text-white transition hover:bg-cyan-700 disabled:cursor-not-allowed disabled:bg-cyan-300"
          >
            {loading ? 'Loading...' : 'Load By Hostname'}
          </button>
          <button
            type="button"
            onClick={save}
            disabled={saving || loading}
            className="rounded-full bg-slate-950 px-4 py-2 text-sm font-semibold text-white transition hover:bg-slate-800 disabled:cursor-not-allowed disabled:bg-slate-400"
          >
            {saving ? 'Publishing...' : 'Publish Full Payload'}
          </button>
        </div>

        <div className="mt-6 rounded-2xl border border-slate-200 bg-slate-50/80 px-4 py-4">
          <p className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">
            Payload Contract
          </p>
          <pre className="mt-3 overflow-x-auto whitespace-pre-wrap break-words font-mono text-xs leading-6 text-slate-700">
            {example}
          </pre>
        </div>

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
      </article>

      <article className="rounded-3xl border border-slate-200/80 bg-white/90 p-6 shadow-sm">
        <div className="flex items-start justify-between gap-4">
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.24em] text-cyan-700">
              Builder JSON
            </p>
            <h2 className="mt-2 text-2xl font-semibold tracking-tight text-slate-950">
              Editor
            </h2>
          </div>
          <span className="rounded-full bg-slate-100 px-3 py-1.5 text-xs font-semibold uppercase tracking-[0.16em] text-slate-700 ring-1 ring-inset ring-slate-200">
            Full Replace
          </span>
        </div>

        <textarea
          value={payload}
          onChange={(event) => setPayload(event.target.value)}
          spellCheck={false}
          className="mt-5 min-h-[42rem] w-full rounded-2xl border border-slate-200 bg-slate-950 p-4 font-mono text-sm leading-6 text-slate-100 outline-none transition focus:border-cyan-500"
        />
      </article>
    </section>
  )
}
