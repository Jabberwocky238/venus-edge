module aaa

go 1.25.3

require (
	capnproto.org/go/capnp/v3 v3.1.0-alpha.2
	github.com/google/uuid v1.6.0
	github.com/miekg/dns v1.1.68
	github.com/oschwald/geoip2-golang v1.13.0
	google.golang.org/grpc v1.79.3
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/colega/zeropool v0.0.0-20230505084239-6fb4a4f75381 // indirect
	github.com/oschwald/maxminddb-golang v1.13.0 // indirect
	golang.org/x/mod v0.30.0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	golang.org/x/tools v0.39.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
)

replace capnproto.org/go/capnp/v3 => /home/jw238/go/pkg/mod/capnproto.org/go/capnp/v3@v3.1.0-alpha.2

replace github.com/colega/zeropool => /home/jw238/go/pkg/mod/github.com/colega/zeropool@v0.0.0-20230505084239-6fb4a4f75381

replace golang.org/x/exp => /home/jw238/go/pkg/mod/golang.org/x/exp@v0.0.0-20251017212417-90e834f514db

replace golang.org/x/sync => /home/jw238/go/pkg/mod/golang.org/x/sync@v0.7.0

replace github.com/oschwald/geoip2-golang => /home/jw238/go/pkg/mod/github.com/oschwald/geoip2-golang@v1.13.0
