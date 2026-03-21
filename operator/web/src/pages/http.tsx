import type { ReactNode } from 'react'
import { useState } from 'react'
import BoardShell from '../components/BoardShell.tsx'
import { type HTTPPayload, publishResource, requestResource } from '../components/api.ts'
import { createHTTPDraftPolicy, createHTTPPayload, type HTTPDraftPolicy } from '../components/http.ts'

export default function HTTPPage() {
  const [hostname, setHostname] = useState('')
  const [policies, setPolicies] = useState<HTTPDraftPolicy[]>([])
  const [creating, setCreating] = useState<HTTPDraftPolicy | null>(null)
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
    setCreating(null)
    try {
      const data = (await requestResource('http', trimmed)) as HTTPPayload
      setPolicies((data.policies || []).map((policy) => createHTTPDraftPolicy(policy)))
      setDirty(false)
      setMessage(`Loaded HTTP policies for ${trimmed}.`)
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
      await publishResource('http', trimmed, JSON.stringify(createHTTPPayload(trimmed, policies)))
      setDirty(false)
      setMessage(`Flushed HTTP policies for ${trimmed}.`)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'flush failed')
    } finally {
      setFlushing(false)
    }
  }

  const patchPolicy = (id: string, patch: Partial<HTTPDraftPolicy>) => {
    setPolicies((current) => current.map((policy) => (policy.id === id ? { ...policy, ...patch } : policy)))
    setDirty(true)
  }

  const removePolicy = (id: string) => {
    setPolicies((current) => current.filter((policy) => policy.id !== id))
    setDirty(true)
  }

  const confirmCreate = () => {
    if (!creating) return
    setPolicies((current) => [creating, ...current])
    setCreating(null)
    setDirty(true)
  }

  return (
    <BoardShell
      title="HTTP Board"
      resource="http"
      description="HTTP resources are managed as a policy board. Each policy is an item that can be edited, removed, or added on top."
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
          <p className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Policies</p>
          <p className="mt-2 font-mono text-3xl font-semibold text-slate-950">{policies.length}</p>
        </div>
      }
    >
      <div className="rounded-3xl border border-slate-200/80 bg-white/90 p-6 shadow-sm">
        <div className="flex items-start justify-between gap-4">
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.24em] text-cyan-700">Policies</p>
            <h2 className="mt-2 text-2xl font-semibold tracking-tight text-slate-950">HTTP Items</h2>
          </div>
          <button type="button" onClick={() => setCreating(createHTTPDraftPolicy())} className="rounded-full bg-slate-950 px-4 py-2 text-sm font-semibold text-white transition hover:bg-slate-800">
            Add Policy
          </button>
        </div>

        <div className="mt-6 space-y-4">
          {creating ? (
            <HTTPPolicyEditor
              title="New Policy"
              policy={creating}
              onChange={(patch) => setCreating((current) => (current ? { ...current, ...patch } : current))}
              onDelete={() => setCreating(null)}
              onConfirm={confirmCreate}
              isCreate
            />
          ) : null}
          {policies.map((policy) => (
            <HTTPPolicyEditor
              key={policy.id}
              title={policy.backend || 'Unnamed Policy'}
              policy={policy}
              onChange={(patch) => patchPolicy(policy.id, patch)}
              onDelete={() => removePolicy(policy.id)}
            />
          ))}
          {policies.length === 0 && !creating ? (
            <div className="rounded-2xl border border-dashed border-slate-300 bg-slate-50 px-4 py-10 text-center text-sm text-slate-500">
              No HTTP policies loaded.
            </div>
          ) : null}
        </div>
      </div>
    </BoardShell>
  )
}

function HTTPPolicyEditor({
  title,
  policy,
  onChange,
  onDelete,
  onConfirm,
  isCreate = false,
}: {
  title: string
  policy: HTTPDraftPolicy
  onChange: (patch: Partial<HTTPDraftPolicy>) => void
  onDelete: () => void
  onConfirm?: () => void
  isCreate?: boolean
}) {
  const mode =
    policy.query_items && policy.query_items.length > 0
      ? 'query'
      : policy.header_items && policy.header_items.length > 0
        ? 'header'
        : 'pathname'

  const setMode = (nextMode: 'pathname' | 'query' | 'header') => {
    if (nextMode === 'pathname') {
      onChange({ query_items: [], header_items: [] })
      return
    }
    if (nextMode === 'query') {
      onChange({ pathname: '', query_items: policy.query_items?.length ? policy.query_items : [{ key: '', value: '' }], header_items: [] })
      return
    }
    onChange({ pathname: '', query_items: [], header_items: policy.header_items?.length ? policy.header_items : [{ key: '', value: '' }] })
  }

  const items = mode === 'query' ? (policy.query_items || []) : (policy.header_items || [])

  return (
    <article className="rounded-2xl border border-slate-200 bg-slate-50/70 p-4">
      <div className="flex items-start justify-between gap-4">
        <h3 className="text-lg font-semibold text-slate-950">{title}</h3>
        <div className="flex gap-2">
          {isCreate ? (
            <>
              <button type="button" onClick={onDelete} className="rounded-full bg-slate-200 px-3 py-1.5 text-xs font-semibold text-slate-700">Cancel</button>
              <button type="button" onClick={onConfirm} className="rounded-full bg-emerald-600 px-3 py-1.5 text-xs font-semibold text-white">Confirm</button>
            </>
          ) : (
            <button type="button" onClick={onDelete} className="rounded-full bg-rose-100 px-3 py-1.5 text-xs font-semibold text-rose-700">Delete</button>
          )}
        </div>
      </div>

      <div className="mt-4 grid gap-4 md:grid-cols-2">
        <Field label="Backend"><input value={policy.backend} onChange={(e) => onChange({ backend: e.target.value })} className={inputClassName} /></Field>
        <Field label="Match Mode">
          <select value={mode} onChange={(e) => setMode(e.target.value as 'pathname' | 'query' | 'header')} className={inputClassName}>
            <option value="pathname">pathname</option>
            <option value="query">query</option>
            <option value="header">header</option>
          </select>
        </Field>
        {mode === 'pathname' ? (
          <>
            <Field label="Path Kind">
              <select value={policy.pathname_kind || 'exact'} onChange={(e) => onChange({ pathname_kind: e.target.value })} className={inputClassName}>
                <option value="exact">exact</option>
                <option value="prefix">prefix</option>
                <option value="regex">regex</option>
              </select>
            </Field>
            <Field label="Pathname" className="md:col-span-2">
              <input value={policy.pathname || ''} onChange={(e) => onChange({ pathname: e.target.value })} className={inputClassName} />
            </Field>
          </>
        ) : (
          <Field label={mode === 'query' ? 'Query Items' : 'Header Items'} className="md:col-span-2">
            <div className="space-y-3">
              {items.map((item, index) => (
                <div key={index} className="grid gap-3 md:grid-cols-[1fr_1fr_auto]">
                  <input
                    value={item.key}
                    onChange={(e) => {
                      const next = items.map((current, currentIndex) => currentIndex === index ? { ...current, key: e.target.value } : current)
                      onChange(mode === 'query' ? { query_items: next } : { header_items: next })
                    }}
                    placeholder="key"
                    className={inputClassName}
                  />
                  <input
                    value={item.value}
                    onChange={(e) => {
                      const next = items.map((current, currentIndex) => currentIndex === index ? { ...current, value: e.target.value } : current)
                      onChange(mode === 'query' ? { query_items: next } : { header_items: next })
                    }}
                    placeholder="value"
                    className={inputClassName}
                  />
                  <button
                    type="button"
                    onClick={() => {
                      const next = items.filter((_, currentIndex) => currentIndex !== index)
                      onChange(mode === 'query' ? { query_items: next } : { header_items: next })
                    }}
                    className="rounded-full bg-rose-100 px-3 py-2 text-xs font-semibold text-rose-700"
                  >
                    Remove
                  </button>
                </div>
              ))}
              <button
                type="button"
                onClick={() => {
                  const next = [...items, { key: '', value: '' }]
                  onChange(mode === 'query' ? { query_items: next } : { header_items: next })
                }}
                className="rounded-full bg-slate-200 px-3 py-2 text-xs font-semibold text-slate-700"
              >
                Add Item
              </button>
            </div>
          </Field>
        )}
      </div>
    </article>
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
