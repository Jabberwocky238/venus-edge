import type { HTTPPolicy, HTTPPayload } from './api.ts'

export type HTTPDraftPolicy = HTTPPolicy & { id: string }

export function createHTTPDraftPolicy(policy?: Partial<HTTPPolicy>): HTTPDraftPolicy {
  return {
    id: crypto.randomUUID(),
    backend: '',
    pathname_kind: 'exact',
    pathname: '',
    query_items: [],
    header_items: [],
    ...policy,
  }
}

export function normalizeHTTPPolicy(policy: HTTPDraftPolicy): HTTPPolicy {
  return {
    backend: policy.backend.trim(),
    pathname_kind: policy.pathname_kind || '',
    pathname: policy.pathname?.trim() || '',
    query_items: (policy.query_items || [])
      .map((item) => ({ key: item.key.trim(), value: item.value.trim() }))
      .filter((item) => item.key || item.value),
    header_items: (policy.header_items || [])
      .map((item) => ({ key: item.key.trim(), value: item.value.trim() }))
      .filter((item) => item.key || item.value),
  }
}

export function createHTTPPayload(name: string, policies: HTTPDraftPolicy[]): HTTPPayload {
  return {
    name: name.trim(),
    policies: policies.map(normalizeHTTPPolicy),
  }
}
