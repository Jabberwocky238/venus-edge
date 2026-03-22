package master

import (
	"bytes"
	"context"
	"log"
	"path/filepath"

	ingress "aaa/ingress"
	ingressbuilder "aaa/ingress/builder"
	acme "aaa/operator/master/acme"
	"aaa/operator/replication"
)

func (m *Master) Root() string {
	return filepath.Join(m.root, "acme")
}

func (m *Master) ReadHTTPRoute(ctx context.Context, hostname string) (*ingressbuilder.HTTPRouteBuilder, error) {
	bin, err := m.readObject(ctx, replication.EventType_EVENT_TYPE_HTTP, hostname)
	if err != nil {
		return nil, err
	}
	zone, err := ingress.ReadHTTPZone(bytes.NewReader(bin))
	if err != nil {
		return nil, err
	}
	route := ingressbuilder.NewHTTPRoute()
	if err := route.From(zone); err != nil {
		return nil, err
	}
	return route, nil
}

func (m *Master) PublishHTTPRoute(ctx context.Context, hostname string, route *ingressbuilder.HTTPRouteBuilder) error {
	bin, err := renderHTTPRoute(route)
	if err != nil {
		return err
	}
	_, err = m.PublishHTTP(ctx, hostname, bin)
	return err
}

func (m *Master) PublishHTTPRouteWithACME(ctx context.Context, hostname string, route *ingressbuilder.HTTPRouteBuilder) (*replication.PushChangeResponse, error) {
	bin, err := renderHTTPRoute(route)
	if err != nil {
		return nil, err
	}
	resp, err := m.PublishHTTP(ctx, hostname, bin)
	if err != nil {
		return nil, err
	}

	routeCopy := ingressbuilder.NewHTTPRoute().Use(route)
	cfg := acme.Config{
		DefaultProvider: m.acme.DefaultProvider,
		DefaultEmail:    m.acme.DefaultEmail,
		ZeroSSLEABKID:   m.acme.ZeroSSLEABKID,
		ZeroSSLEABHMAC:  m.acme.ZeroSSLEABHMAC,
	}
	go func() {
		if err := acme.HandleHTTPPublish(context.Background(), m, cfg, hostname, routeCopy); err != nil {
			log.Printf("%s %sacme async%s hostname=%s err=%v", masterLogPrefix, masterLogFail, masterLogReset, hostname, err)
		}
	}()

	return resp, nil
}

func (m *Master) ReadTLSRoute(ctx context.Context, hostname string) (*ingressbuilder.TLSRouteBuilder, error) {
	bin, err := m.readObject(ctx, replication.EventType_EVENT_TYPE_TLS, hostname)
	if err != nil {
		return nil, err
	}
	zone, err := ingress.ReadTLSZone(bytes.NewReader(bin))
	if err != nil {
		return nil, err
	}
	route := ingressbuilder.NewTLSRoute()
	if err := route.From(zone); err != nil {
		return nil, err
	}
	return route, nil
}

func (m *Master) PublishTLSRoute(ctx context.Context, hostname string, route *ingressbuilder.TLSRouteBuilder) error {
	bin, err := renderTLSRoute(route)
	if err != nil {
		return err
	}
	_, err = m.PublishTLS(ctx, hostname, bin)
	return err
}

func (m *Master) PublishTLSRouteWithResponse(ctx context.Context, hostname string, route *ingressbuilder.TLSRouteBuilder) (*replication.PushChangeResponse, error) {
	bin, err := renderTLSRoute(route)
	if err != nil {
		return nil, err
	}
	return m.PublishTLS(ctx, hostname, bin)
}
