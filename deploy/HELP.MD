```bash
curl -X PUT 'http://127.0.0.1:9000/api/master/http?hostname=venus-edge.app238.com' -H 'Content-Type: application/json' --data-raw '{"name": "venus-edge-manage", "policies": [{ "backend": "http://venus-master.venus-edge.svc.cluster.local:9000", "pathname_kind": "prefix","pathname": "/","allow_raw_access": true}]}'
```