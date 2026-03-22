import type { DNSRecord } from './api.ts'
import { createID } from './id.ts'

export type DNSDraftRecord = DNSRecord & { id: string }

export const dnsRecordTypes = ['a', 'aaaa', 'cname', 'mx', 'ns', 'ptr', 'soa', 'txt']

export function createDNSDraft(record?: Partial<DNSRecord>): DNSDraftRecord {
  return {
    id: createID(),
    type: 'a',
    name: '',
    ttl: 300,
    values: [],
    ...record,
  }
}

export function normalizeDNSRecord(record: DNSDraftRecord): DNSRecord {
  const next: DNSRecord = {
    type: record.type,
    name: record.name.trim(),
  }
  if (record.ttl) next.ttl = Number(record.ttl)
  if (record.address?.trim()) next.address = record.address.trim()
  if (record.host?.trim()) next.host = record.host.trim()
  if (record.values?.length) next.values = record.values.map((item) => item.trim()).filter(Boolean)
  if (record.preference) next.preference = Number(record.preference)
  if (record.exchange?.trim()) next.exchange = record.exchange.trim()
  if (record.mname?.trim()) next.mname = record.mname.trim()
  if (record.rname?.trim()) next.rname = record.rname.trim()
  if (record.serial) next.serial = Number(record.serial)
  if (record.refresh) next.refresh = Number(record.refresh)
  if (record.retry) next.retry = Number(record.retry)
  if (record.expire) next.expire = Number(record.expire)
  if (record.minimum) next.minimum = Number(record.minimum)
  return next
}

export function dnsRecordSummary(record: DNSDraftRecord) {
  switch (record.type) {
    case 'a':
    case 'aaaa':
      return record.address || 'address'
    case 'cname':
    case 'ns':
    case 'ptr':
      return record.host || 'host'
    case 'mx':
      return `${record.preference ?? 0} ${record.exchange || 'exchange'}`
    case 'soa':
      return `${record.mname || 'mname'} / ${record.rname || 'rname'}`
    case 'txt':
      return record.values?.join(', ') || 'values'
    default:
      return ''
  }
}
