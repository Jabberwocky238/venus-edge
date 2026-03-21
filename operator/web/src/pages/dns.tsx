import ResourcePage from './resource-page.tsx'

const example = `{
  "records": [
    {
      "type": "a",
      "name": "example.com",
      "ttl": 300,
      "address": "1.2.3.4"
    },
    {
      "type": "txt",
      "name": "_acme-challenge.example.com",
      "ttl": 60,
      "values": ["token-value"]
    }
  ]
}`

export default function DNSPage() {
  return (
    <ResourcePage
      title="DNS Builder Board"
      resource="dns"
      description="Loads every DNS record stored under the hostname, exposes the full DNS builder JSON, and publishes the complete record set back to master."
      example={example}
    />
  )
}
