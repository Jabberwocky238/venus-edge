package master

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	dns "aaa/DNS"
	dnsbuilder "aaa/DNS/builder"
	ingress "aaa/ingress"
	ingressbuilder "aaa/ingress/builder"
	"aaa/operator/replication"
)

type DNSChangeJSON struct {
	Records []DNSRecordJSON `json:"records"`
}

type DNSRecordJSON struct {
	Type       string   `json:"type"`
	Name       string   `json:"name"`
	TTL        uint32   `json:"ttl,omitempty"`
	Address    string   `json:"address,omitempty"`
	Host       string   `json:"host,omitempty"`
	Values     []string `json:"values,omitempty"`
	Preference uint16   `json:"preference,omitempty"`
	Exchange   string   `json:"exchange,omitempty"`
	MName      string   `json:"mname,omitempty"`
	RName      string   `json:"rname,omitempty"`
	Serial     uint32   `json:"serial,omitempty"`
	Refresh    uint32   `json:"refresh,omitempty"`
	Retry      uint32   `json:"retry,omitempty"`
	Expire     uint32   `json:"expire,omitempty"`
	Minimum    uint32   `json:"minimum,omitempty"`
}

type TLSChangeJSON struct {
	Name            string `json:"name"`
	SNI             string `json:"sni"`
	CertPEM         string `json:"cert_pem,omitempty"`
	KeyPEM          string `json:"key_pem,omitempty"`
	Kind            string `json:"kind,omitempty"`
	BackendHostname string `json:"backend_hostname,omitempty"`
	BackendPort     uint16 `json:"backend_port,omitempty"`
}

type HTTPChangeJSON struct {
	Name     string           `json:"name"`
	Policies []HTTPPolicyJSON `json:"policies"`
}

type HTTPPolicyJSON struct {
	Backend      string       `json:"backend"`
	PathnameKind string       `json:"pathname_kind,omitempty"`
	Pathname     string       `json:"pathname,omitempty"`
	QueryItems   []HTTPKVJSON `json:"query_items,omitempty"`
	HeaderItems  []HTTPKVJSON `json:"header_items,omitempty"`
}

type HTTPKVJSON struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (m *Master) PublishDNSJSON(ctx context.Context, hostname string, payload []byte) (*replication.PushChangeResponse, error) {
	var change DNSChangeJSON
	if err := json.Unmarshal(payload, &change); err != nil {
		return nil, fmt.Errorf("decode dns builder json: %w", err)
	}
	bin, err := renderDNSChange(change)
	if err != nil {
		return nil, err
	}
	return m.PublishDNS(ctx, hostname, bin)
}

func (m *Master) PublishTLSJSON(ctx context.Context, hostname string, payload []byte) (*replication.PushChangeResponse, error) {
	var change TLSChangeJSON
	if err := json.Unmarshal(payload, &change); err != nil {
		return nil, fmt.Errorf("decode tls builder json: %w", err)
	}
	bin, err := renderTLSChange(change)
	if err != nil {
		return nil, err
	}
	return m.PublishTLS(ctx, hostname, bin)
}

func (m *Master) PublishHTTPJSON(ctx context.Context, hostname string, payload []byte) (*replication.PushChangeResponse, error) {
	var change HTTPChangeJSON
	if err := json.Unmarshal(payload, &change); err != nil {
		return nil, fmt.Errorf("decode http builder json: %w", err)
	}
	bin, err := renderHTTPChange(change)
	if err != nil {
		return nil, err
	}
	return m.PublishHTTP(ctx, hostname, bin)
}

func renderDNSChange(change DNSChangeJSON) ([]byte, error) {
	records := make([]dns.RecordBuilder, 0, len(change.Records))
	for i, record := range change.Records {
		b, err := buildDNSRecord(record)
		if err != nil {
			return nil, fmt.Errorf("build dns record %d: %w", i, err)
		}
		records = append(records, b)
	}

	var buf bytes.Buffer
	if err := writeDNSRecords(&buf, records...); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func renderTLSChange(change TLSChangeJSON) ([]byte, error) {
	route, err := buildTLSRoute(change)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := writeTLSZone(&buf, route); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func renderHTTPChange(change HTTPChangeJSON) ([]byte, error) {
	route, err := buildHTTPRoute(change)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := writeHTTPZone(&buf, route); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildDNSRecord(record DNSRecordJSON) (dns.RecordBuilder, error) {
	switch record.Type {
	case "a":
		builder := dnsbuilder.NewA().
			WithName(record.Name).
			WithAddress(record.Address)
		if record.TTL != 0 {
			builder.WithTTL(record.TTL)
		}
		return builder, nil
	case "aaaa":
		builder := dnsbuilder.NewAAAA().
			WithName(record.Name).
			WithAddress(record.Address)
		if record.TTL != 0 {
			builder.WithTTL(record.TTL)
		}
		return builder, nil
	case "cname":
		builder := dnsbuilder.NewCNAME().
			WithName(record.Name).
			WithHost(record.Host)
		if record.TTL != 0 {
			builder.WithTTL(record.TTL)
		}
		return builder, nil
	case "mx":
		builder := dnsbuilder.NewMX().
			WithName(record.Name).
			WithPreference(record.Preference).
			WithExchange(record.Exchange)
		if record.TTL != 0 {
			builder.WithTTL(record.TTL)
		}
		return builder, nil
	case "ns":
		builder := dnsbuilder.NewNS().
			WithName(record.Name).
			WithHost(record.Host)
		if record.TTL != 0 {
			builder.WithTTL(record.TTL)
		}
		return builder, nil
	case "ptr":
		builder := dnsbuilder.NewPTR().
			WithName(record.Name).
			WithHost(record.Host)
		if record.TTL != 0 {
			builder.WithTTL(record.TTL)
		}
		return builder, nil
	case "soa":
		builder := dnsbuilder.NewSOA().
			WithName(record.Name).
			WithMName(record.MName).
			WithRName(record.RName)
		if record.TTL != 0 {
			builder.WithTTL(record.TTL)
		}
		if record.Serial != 0 {
			builder.WithSerial(record.Serial)
		}
		if record.Refresh != 0 {
			builder.WithRefresh(record.Refresh)
		}
		if record.Retry != 0 {
			builder.WithRetry(record.Retry)
		}
		if record.Expire != 0 {
			builder.WithExpire(record.Expire)
		}
		if record.Minimum != 0 {
			builder.WithMinimum(record.Minimum)
		}
		return builder, nil
	case "txt":
		builder := dnsbuilder.NewTXT().
			WithName(record.Name).
			WithValues(record.Values...)
		if record.TTL != 0 {
			builder.WithTTL(record.TTL)
		}
		return builder, nil
	default:
		return nil, fmt.Errorf("unsupported dns record type: %q", record.Type)
	}
}

func buildTLSRoute(change TLSChangeJSON) (ingress.TLSRoute, error) {
	builder := ingressbuilder.NewTLSRoute().
		WithName(change.Name).
		WithSNI(change.SNI).
		WithCertPEM(change.CertPEM).
		WithKeyPEM(change.KeyPEM)

	kind, err := parseTLSKind(change.Kind)
	if err != nil {
		return nil, err
	}
	builder.WithKind(kind)
	if change.BackendHostname != "" || change.BackendPort != 0 {
		builder.WithBackend(change.BackendHostname, change.BackendPort)
	}
	return builder, nil
}

func buildHTTPRoute(change HTTPChangeJSON) (ingress.HTTPRoute, error) {
	route := ingressbuilder.NewHTTPRoute().WithName(change.Name)
	for i, policy := range change.Policies {
		next, err := buildHTTPPolicy(policy)
		if err != nil {
			return nil, fmt.Errorf("build http policy %d: %w", i, err)
		}
		route.AddPolicy(next)
	}
	return route, nil
}

func buildHTTPPolicy(policy HTTPPolicyJSON) (*ingressbuilder.HTTPPolicyBuilder, error) {
	builder := ingressbuilder.NewHTTPPolicy().WithBackend(policy.Backend)

	switch {
	case policy.Pathname != "":
		switch policy.PathnameKind {
		case "exact", "":
			builder.WithExactPath(policy.Pathname)
		case "prefix":
			builder.WithPrefixPath(policy.Pathname)
		case "regex":
			builder.WithRegexPath(policy.Pathname)
		default:
			return nil, fmt.Errorf("unsupported pathname_kind: %q", policy.PathnameKind)
		}
	case len(policy.QueryItems) > 0:
		for _, item := range policy.QueryItems {
			builder.WithQuery(item.Key, item.Value)
		}
	case len(policy.HeaderItems) > 0:
		for _, item := range policy.HeaderItems {
			builder.WithHeader(item.Key, item.Value)
		}
	default:
		return nil, fmt.Errorf("one of pathname, query_items, header_items is required")
	}

	return builder, nil
}

func parseTLSKind(kind string) (ingress.TlsPolicy_Kind, error) {
	switch kind {
	case "", "https":
		return ingress.TlsPolicy_Kind_https, nil
	case "tlsPassthrough":
		return ingress.TlsPolicy_Kind_tlsPassthrough, nil
	case "tlsTerminate":
		return ingress.TlsPolicy_Kind_tlsTerminate, nil
	default:
		return 0, fmt.Errorf("unsupported tls kind: %q", kind)
	}
}

func writeDNSRecords(buf *bytes.Buffer, records ...dns.RecordBuilder) error {
	dir, err := os.MkdirTemp("", "master-dns-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "zone.bin")
	if err := dns.Write(path, records...); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	buf.Reset()
	_, _ = buf.Write(data)
	return nil
}

func writeTLSZone(buf *bytes.Buffer, route ingress.TLSRoute) error {
	dir, err := os.MkdirTemp("", "master-tls-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "zone.bin")
	if err := ingress.WriteTLSZone(path, route); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	buf.Reset()
	_, _ = buf.Write(data)
	return nil
}

func writeHTTPZone(buf *bytes.Buffer, route ingress.HTTPRoute) error {
	dir, err := os.MkdirTemp("", "master-http-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "zone.bin")
	if err := ingress.WriteHTTPZone(path, route); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	buf.Reset()
	_, _ = buf.Write(data)
	return nil
}

func envelopeTimestampUnix() int64 {
	return time.Now().Unix()
}
