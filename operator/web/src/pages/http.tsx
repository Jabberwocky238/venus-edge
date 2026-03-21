import ResourcePage from './resource-page.tsx'

const example = `{
  "name": "example.com",
  "policies": [
    {
      "backend": "svc.default:8080",
      "pathname_kind": "exact",
      "pathname": "/"
    }
  ]
}`

export default function HTTPPage() {
  return (
    <ResourcePage
      title="HTTP Builder Board"
      resource="http"
      description="Loads every HTTP policy bound to the hostname, exposes the full HTTP builder JSON, and publishes the complete hostname payload back to master."
      example={example}
    />
  )
}
