import ResourcePage from './resource-page.tsx'

const example = `{
  "name": "example.com",
  "sni": "example.com",
  "kind": "https",
  "cert_pem": "-----BEGIN CERTIFICATE-----\\n...\\n-----END CERTIFICATE-----",
  "key_pem": "-----BEGIN PRIVATE KEY-----\\n...\\n-----END PRIVATE KEY-----",
  "backend_hostname": "",
  "backend_port": 0
}`

export default function TLSPage() {
  return (
    <ResourcePage
      title="TLS Builder Board"
      resource="tls"
      description="Loads the TLS resource for the hostname, lets you edit the full TLS builder payload, and republishes that complete payload through master."
      example={example}
    />
  )
}
