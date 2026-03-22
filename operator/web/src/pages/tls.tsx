import type { ReactNode } from 'react'
import { useState } from 'react'
import BoardShell from '../components/BoardShell.tsx'
import { type TLSPayload, publishResource, requestResource } from '../components/api.ts'
import { createTLSDraft, normalizeTLSDraft, tlsKinds, type TLSDraft } from '../components/tls.ts'

export default function TLSPage() {
  const [hostname, setHostname] = useState('')
  const [draft, setDraft] = useState<TLSDraft>(createTLSDraft())
  const [loading, setLoading] = useState(false)
  const [flushing, setFlushing] = useState(false)
  const [dirty, setDirty] = useState(false)
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
      const data = (await requestResource('tls', trimmed)) as TLSPayload
      setDraft(createTLSDraft({ ...data, name: trimmed, sni: trimmed }))
      setDirty(false)
      setMessage(`Loaded TLS resource for ${trimmed}.`)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'load failed')
    } finally {
      setLoading(false)
    }
  }

  const flush = async () => {
    const trimmed = hostname.trim()
    if (!trimmed) {
      setError('hostname is required')
      return
    }
    setFlushing(true)
    setError(null)
    setMessage(null)
    try {
      await publishResource(
        'tls',
        trimmed,
        JSON.stringify(
          normalizeTLSDraft({
            ...draft,
            name: trimmed,
            sni: trimmed,
          }),
        ),
      )
      setDirty(false)
      setMessage(`Flushed TLS resource for ${trimmed}.`)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'flush failed')
    } finally {
      setFlushing(false)
    }
  }

  const patch = (next: Partial<TLSDraft>) => {
    setDraft((current) => ({ ...current, ...next }))
    setDirty(true)
  }

  return (
    <BoardShell
      title="TLS Board"
      resource="tls"
      description="TLS resource is edited as a single builder board with certificate, key, SNI, kind, and optional backend."
      hostname={hostname}
      onHostnameChange={setHostname}
      onLoad={load}
      onFlush={flush}
      loading={loading}
      flushing={flushing}
      dirty={dirty}
      message={message}
      error={error}
      sidebarExtra={
        <div className="rounded-2xl border border-slate-200 bg-slate-50/80 px-4 py-4">
          <p className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Kind</p>
          <p className="mt-2 font-mono text-lg font-semibold text-slate-950">{draft.kind || 'n/a'}</p>
        </div>
      }
    >
      <div className="rounded-3xl border border-slate-200/80 bg-white/90 p-6 shadow-sm">
        <div className="flex items-start justify-between gap-4">
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.24em] text-cyan-700">Policy</p>
            <h2 className="mt-2 text-2xl font-semibold tracking-tight text-slate-950">TLS Resource</h2>
          </div>
        </div>

        <div className="mt-6 grid gap-4 md:grid-cols-2">
          {/* <Field label="Name">
            <input value={hostname.trim()} readOnly className={readOnlyInputClassName} />
          </Field>
          <Field label="SNI">
            <input value={hostname.trim()} readOnly className={readOnlyInputClassName} />
          </Field> */}
          <Field label="Kind">
            <select value={draft.kind} onChange={(e) => patch({ kind: e.target.value })} className={inputClassName}>
              {tlsKinds.map((kind) => <option key={kind} value={kind}>{kind}</option>)}
            </select>
          </Field>
          <Field label="Backend Port"><input value={draft.backend_port ?? ''} onChange={(e) => patch({ backend_port: Number(e.target.value) || 0 })} className={inputClassName} /></Field>
          <Field label="Backend Hostname (eg: 127.0.0.1 or api.example.com)" className="md:col-span-2"><input value={draft.backend_hostname || ''} onChange={(e) => patch({ backend_hostname: e.target.value })} className={inputClassName} /></Field>
          <Field label="Certificate PEM" className="md:col-span-2">
            <textarea value={draft.cert_pem || ''} onChange={(e) => patch({ cert_pem: e.target.value })} className={`${inputClassName} min-h-44`} />
          </Field>
          <Field label="Key PEM" className="md:col-span-2">
            <textarea value={draft.key_pem || ''} onChange={(e) => patch({ key_pem: e.target.value })} className={`${inputClassName} min-h-44`} />
          </Field>
        </div>
      </div>
    </BoardShell>
  )
}

function Field({ label, children, className = '' }: { label: string; children: ReactNode; className?: string }) {
  return (
    <label className={`block ${className}`}>
      <span className="mb-2 block text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">{label}</span>
      {children}
    </label>
  )
}

const inputClassName =
  'w-full rounded-2xl border border-slate-200 bg-white px-4 py-3 font-mono text-sm text-slate-900 outline-none transition focus:border-cyan-500'

const readOnlyInputClassName =
  'w-full rounded-2xl border border-slate-200 bg-slate-100 px-4 py-3 font-mono text-sm text-slate-500 outline-none'
