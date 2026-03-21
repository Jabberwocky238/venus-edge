run-agent:
	go run operator/agent/cmd/main.go

run-master:
	go run operator/master/cmd/main.go

dev-web:
	cd operator/web && bun run dev