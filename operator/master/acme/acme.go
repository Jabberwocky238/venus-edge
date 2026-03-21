package acme

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	dns "aaa/DNS"
	dnsbuilder "aaa/DNS/builder"
	ingress "aaa/ingress"
	ingressbuilder "aaa/ingress/builder"
	master "aaa/operator/master"
)

type Manager struct {
	master *master.Master
}

func New(m *master.Master) *Manager {
	return &Manager{master: m}
}

func (m *Manager) DNS01() *DNS01Solver {
	return &DNS01Solver{master: m.master}
}

func (m *Manager) HTTP01() *HTTP01Solver {
	return &HTTP01Solver{master: m.master}
}

func renderDNSBin(route dns.RecordBuilder) ([]byte, error) {
	dir, err := os.MkdirTemp("", "acme-dns-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "zone.bin")
	if err := dns.Write(path, route); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func renderHTTPBin(route ingress.HTTPRoute) ([]byte, error) {
	dir, err := os.MkdirTemp("", "acme-http-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "zone.bin")
	if err := ingress.WriteHTTPZone(path, route); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func ensureMaster(m *master.Master) error {
	if m == nil {
		return fmt.Errorf("master is required")
	}
	return nil
}

func newTXTRecord(name, value string) dns.RecordBuilder {
	return dnsbuilder.NewTXT().WithName(name).WithValues(value)
}

func newHTTPRoute(hostname, token, backend string) ingress.HTTPRoute {
	return ingressbuilder.NewHTTPRoute().
		WithName(hostname).
		AddPolicy(
			ingressbuilder.NewHTTPPolicy().
				WithBackend(backend).
				WithExactPath("/.well-known/acme-challenge/" + token),
		)
}

func run(ctx context.Context, fn func(context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return fn(ctx)
}
