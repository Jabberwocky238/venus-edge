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
    use_fix_content: false,
    fix_content: '',
    allow_raw_access: false,
    ...policy,
  }
}

export function normalizeHTTPPolicy(policy: HTTPDraftPolicy): HTTPPolicy {
  return {
    backend: policy.use_fix_content ? '' : policy.backend.trim(),
    pathname_kind: policy.pathname_kind || '',
    pathname: policy.pathname?.trim() || '',
    use_fix_content: Boolean(policy.use_fix_content),
    fix_content: policy.use_fix_content ? policy.fix_content?.trim() || '' : '',
    allow_raw_access: Boolean(policy.allow_raw_access),
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
