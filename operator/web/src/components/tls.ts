import type { TLSPayload } from './api.ts'

export type TLSDraft = TLSPayload

export const tlsKinds = ['https', 'tlsPassthrough', 'tlsTerminate']

export function createTLSDraft(payload?: Partial<TLSPayload>): TLSDraft {
  return {
    name: '',
    sni: '',
    kind: 'https',
    cert_pem: '',
    key_pem: '',
    backend_hostname: '',
    backend_port: 0,
    ...payload,
  }
}

export function normalizeTLSDraft(draft: TLSDraft): TLSPayload {
  return {
    name: draft.name.trim(),
    sni: draft.sni.trim(),
    kind: draft.kind,
    cert_pem: draft.cert_pem?.trim() || '',
    key_pem: draft.key_pem?.trim() || '',
    backend_hostname: draft.backend_hostname?.trim() || '',
    backend_port: Number(draft.backend_port) || 0,
  }
}
