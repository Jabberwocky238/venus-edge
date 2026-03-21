import type { ReactNode } from 'react'
import { useState } from 'react'
import BoardShell from '../components/BoardShell.tsx'
import {
  type DNSPayload,
  publishResource,
  requestResource,
} from '../components/api.ts'
import {
  createDNSDraft,
  dnsRecordSummary,
  dnsRecordTypes,
  normalizeDNSRecord,
  type DNSDraftRecord,
} from '../components/dns.ts'

export default function DNSPage() {
  const [hostname, setHostname] = useState('')
  const [records, setRecords] = useState<DNSDraftRecord[]>([])
  const [loading, setLoading] = useState(false)
  const [flushing, setFlushing] = useState(false)
  const [dirty, setDirty] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [message, setMessage] = useState<string | null>(null)
  const [creating, setCreating] = useState<DNSDraftRecord | null>(null)

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
      const data = (await requestResource('dns', trimmed)) as DNSPayload
      setRecords((data.records || []).map((record) => createDNSDraft(record)))
      setDirty(false)
      setMessage(`Loaded DNS records for ${trimmed}.`)
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
      await publishResource('dns', trimmed, JSON.stringify({
        records: records.map(normalizeDNSRecord),
      }))
      setDirty(false)
      setMessage(`Flushed DNS records for ${trimmed}.`)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'flush failed')
    } finally {
      setFlushing(false)
    }
  }

  const updateRecord = (id: string, patch: Partial<DNSDraftRecord>) => {
    setRecords((current) => current.map((record) => (record.id === id ? { ...record, ...patch } : record)))
    setDirty(true)
  }

  const removeRecord = (id: string) => {
    setRecords((current) => current.filter((record) => record.id !== id))
    setDirty(true)
  }

  const addRecord = () => {
    setCreating(createDNSDraft())
  }

  const confirmCreate = () => {
    if (!creating) return
    setRecords((current) => [creating, ...current])
    setCreating(null)
    setDirty(true)
  }

  return (
    <BoardShell
      title="DNS Board"
      resource="dns"
      description="Left side controls hostname and flush. Right side manages all DNS records under that hostname as editable items."
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
          <p className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">
            Loaded Records
          </p>
          <p className="mt-2 font-mono text-3xl font-semibold text-slate-950">{records.length}</p>
        </div>
      }
    >
      <div className="rounded-3xl border border-slate-200/80 bg-white/90 p-6 shadow-sm">
        <div className="flex items-start justify-between gap-4">
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.24em] text-cyan-700">Records</p>
            <h2 className="mt-2 text-2xl font-semibold tracking-tight text-slate-950">DNS Items</h2>
          </div>
          <button
            type="button"
            onClick={addRecord}
            className="rounded-full bg-slate-950 px-4 py-2 text-sm font-semibold text-white transition hover:bg-slate-800"
          >
            Add Record
          </button>
        </div>

        <div className="mt-6 space-y-4">
          {creating ? (
            <RecordEditor
              title="New Record"
              record={creating}
              onChange={(patch) => setCreating((current) => (current ? { ...current, ...patch } : current))}
              onDelete={() => setCreating(null)}
              onConfirm={confirmCreate}
              isCreate
            />
          ) : null}

          {records.map((record) => (
            <RecordEditor
              key={record.id}
              title={record.name || 'Unnamed Record'}
              subtitle={dnsRecordSummary(record)}
              record={record}
              onChange={(patch) => updateRecord(record.id, patch)}
              onDelete={() => removeRecord(record.id)}
            />
          ))}

          {records.length === 0 && !creating ? (
            <div className="rounded-2xl border border-dashed border-slate-300 bg-slate-50 px-4 py-10 text-center text-sm text-slate-500">
              No DNS records loaded.
            </div>
          ) : null}
        </div>
      </div>
    </BoardShell>
  )
}

function RecordEditor({
  title,
  subtitle,
  record,
  onChange,
  onDelete,
  onConfirm,
  isCreate = false,
}: {
  title: string
  subtitle?: string
  record: DNSDraftRecord
  onChange: (patch: Partial<DNSDraftRecord>) => void
  onDelete: () => void
  onConfirm?: () => void
  isCreate?: boolean
}) {
  const values = (record.values || []).join('\n')

  return (
    <article className="rounded-2xl border border-slate-200 bg-slate-50/70 p-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h3 className="text-lg font-semibold text-slate-950">{title}</h3>
          {subtitle ? <p className="mt-1 text-sm text-slate-500">{subtitle}</p> : null}
        </div>
        <div className="flex gap-2">
          {isCreate ? (
            <>
              <button type="button" onClick={onDelete} className="rounded-full bg-slate-200 px-3 py-1.5 text-xs font-semibold text-slate-700">
                Cancel
              </button>
              <button type="button" onClick={onConfirm} className="rounded-full bg-emerald-600 px-3 py-1.5 text-xs font-semibold text-white">
                Confirm
              </button>
            </>
          ) : (
            <button type="button" onClick={onDelete} className="rounded-full bg-rose-100 px-3 py-1.5 text-xs font-semibold text-rose-700">
              Delete
            </button>
          )}
        </div>
      </div>

      <div className="mt-4 grid gap-4 md:grid-cols-2">
        <Field label="Type">
          <select value={record.type} onChange={(event) => onChange({ type: event.target.value })} className={inputClassName}>
            {dnsRecordTypes.map((item) => <option key={item} value={item}>{item}</option>)}
          </select>
        </Field>
        <Field label="TTL">
          <input value={record.ttl ?? ''} onChange={(event) => onChange({ ttl: Number(event.target.value) || 0 })} className={inputClassName} />
        </Field>
        <Field label="Name">
          <input value={record.name} onChange={(event) => onChange({ name: event.target.value })} className={inputClassName} />
        </Field>
        {(record.type === 'a' || record.type === 'aaaa') ? (
          <Field label="Address">
            <input value={record.address || ''} onChange={(event) => onChange({ address: event.target.value })} className={inputClassName} />
          </Field>
        ) : null}
        {['cname', 'ns', 'ptr'].includes(record.type) ? (
          <Field label="Host">
            <input value={record.host || ''} onChange={(event) => onChange({ host: event.target.value })} className={inputClassName} />
          </Field>
        ) : null}
        {record.type === 'mx' ? (
          <>
            <Field label="Preference">
              <input value={record.preference ?? ''} onChange={(event) => onChange({ preference: Number(event.target.value) || 0 })} className={inputClassName} />
            </Field>
            <Field label="Exchange">
              <input value={record.exchange || ''} onChange={(event) => onChange({ exchange: event.target.value })} className={inputClassName} />
            </Field>
          </>
        ) : null}
        {record.type === 'soa' ? (
          <>
            <Field label="MName"><input value={record.mname || ''} onChange={(event) => onChange({ mname: event.target.value })} className={inputClassName} /></Field>
            <Field label="RName"><input value={record.rname || ''} onChange={(event) => onChange({ rname: event.target.value })} className={inputClassName} /></Field>
            <Field label="Serial"><input value={record.serial ?? ''} onChange={(event) => onChange({ serial: Number(event.target.value) || 0 })} className={inputClassName} /></Field>
            <Field label="Refresh"><input value={record.refresh ?? ''} onChange={(event) => onChange({ refresh: Number(event.target.value) || 0 })} className={inputClassName} /></Field>
            <Field label="Retry"><input value={record.retry ?? ''} onChange={(event) => onChange({ retry: Number(event.target.value) || 0 })} className={inputClassName} /></Field>
            <Field label="Expire"><input value={record.expire ?? ''} onChange={(event) => onChange({ expire: Number(event.target.value) || 0 })} className={inputClassName} /></Field>
            <Field label="Minimum"><input value={record.minimum ?? ''} onChange={(event) => onChange({ minimum: Number(event.target.value) || 0 })} className={inputClassName} /></Field>
          </>
        ) : null}
      </div>

      {record.type === 'txt' ? (
        <Field label="Values" className="mt-4">
          <textarea
            value={values}
            onChange={(event) => onChange({ values: event.target.value.split('\n') })}
            className={`${inputClassName} min-h-28`}
          />
        </Field>
      ) : null}
    </article>
  )
}

function Field({
  label,
  children,
  className = '',
}: {
  label: string
  children: ReactNode
  className?: string
}) {
  return (
    <label className={`block ${className}`}>
      <span className="mb-2 block text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">{label}</span>
      {children}
    </label>
  )
}

const inputClassName =
  'w-full rounded-2xl border border-slate-200 bg-white px-4 py-3 font-mono text-sm text-slate-900 outline-none transition focus:border-cyan-500'
