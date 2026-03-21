export type ResourceKind = 'dns' | 'tls' | 'http'

export type PublishResult = {
  accepted?: boolean
  message?: string
}

export type DNSRecord = {
  type: string
  name: string
  ttl?: number
  address?: string
  host?: string
  values?: string[]
  preference?: number
  exchange?: string
  mname?: string
  rname?: string
  serial?: number
  refresh?: number
  retry?: number
  expire?: number
  minimum?: number
}

export type DNSPayload = {
  records: DNSRecord[]
}

export type TLSPayload = {
  name: string
  sni: string
  kind: string
  cert_pem?: string
  key_pem?: string
  backend_hostname?: string
  backend_port?: number
}

export type HTTPKeyValue = {
  key: string
  value: string
}

export type HTTPPolicy = {
  backend: string
  pathname_kind?: string
  pathname?: string
  query_items?: HTTPKeyValue[]
  header_items?: HTTPKeyValue[]
}

export type HTTPPayload = {
  name: string
  policies: HTTPPolicy[]
}

function getApiBase() {
  if (typeof window !== 'undefined' && window.location.hostname === 'localhost') {
    return 'http://127.0.0.1:9000'
  }
  return ''
}

function buildURL(path: string, params?: URLSearchParams) {
  const base = getApiBase()
  const query = params?.toString()
  return `${base}${path}${query ? `?${query}` : ''}`
}

async function readTextError(response: Response) {
  const text = await response.text()
  return text || `${response.status} ${response.statusText}`
}

export async function requestResource(resource: ResourceKind, hostname: string) {
  const response = await fetch(
    buildURL(
      `/api/master/${resource}`,
      new URLSearchParams({ hostname }),
    ),
  )
  if (!response.ok) {
    throw new Error(await readTextError(response))
  }
  return response.json()
}

export async function publishResource(
  resource: ResourceKind,
  hostname: string,
  payload: string,
) {
  const response = await fetch(
    buildURL(
      `/api/master/${resource}`,
      new URLSearchParams({ hostname }),
    ),
    {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
      },
      body: payload,
    },
  )
  if (!response.ok) {
    throw new Error(await readTextError(response))
  }
  return (await response.json()) as PublishResult
}
