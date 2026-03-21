package master

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"

	dns "aaa/DNS"
	ingress "aaa/ingress"
	"aaa/operator/replication"
)

func (m *Master) ReadDNSJSON(ctx context.Context, hostname string) (DNSChangeJSON, error) {
	bin, err := m.readObject(ctx, replication.EventType_EVENT_TYPE_DNS, hostname)
	if err != nil {
		return DNSChangeJSON{}, err
	}
	return decodeDNSChange(bin)
}

func (m *Master) ReadTLSJSON(ctx context.Context, hostname string) (TLSChangeJSON, error) {
	bin, err := m.readObject(ctx, replication.EventType_EVENT_TYPE_TLS, hostname)
	if err != nil {
		return TLSChangeJSON{}, err
	}
	return decodeTLSChange(bin)
}

func (m *Master) ReadHTTPJSON(ctx context.Context, hostname string) (HTTPChangeJSON, error) {
	bin, err := m.readObject(ctx, replication.EventType_EVENT_TYPE_HTTP, hostname)
	if err != nil {
		return HTTPChangeJSON{}, err
	}
	return decodeHTTPChange(bin)
}

func (m *Master) readObject(ctx context.Context, kind replication.EventType, hostname string) ([]byte, error) {
	key, err := objectKey(kind, hostname)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := m.store.Get(ctx, key, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeDNSChange(bin []byte) (DNSChangeJSON, error) {
	zone, err := dns.Read(bytes.NewReader(bin))
	if err != nil {
		return DNSChangeJSON{}, err
	}
	records, err := zone.Records()
	if err != nil {
		return DNSChangeJSON{}, err
	}

	change := DNSChangeJSON{
		Records: make([]DNSRecordJSON, 0, records.Len()),
	}
	for i := 0; i < records.Len(); i++ {
		record, err := decodeDNSRecord(records.At(i))
		if err != nil {
			return DNSChangeJSON{}, fmt.Errorf("decode dns record %d: %w", i, err)
		}
		change.Records = append(change.Records, record)
	}
	return change, nil
}

func decodeDNSRecord(record dns.DnsRecord) (DNSRecordJSON, error) {
	name, err := record.Name()
	if err != nil {
		return DNSRecordJSON{}, err
	}

	out := DNSRecordJSON{
		Type: record.Type().String(),
		Name: name,
		TTL:  record.Ttl(),
	}

	switch record.Which() {
	case dns.DnsRecord_Which_a:
		value, err := record.A()
		if err != nil {
			return DNSRecordJSON{}, err
		}
		out.Address = net.IPv4(
			byte(value.Address()>>24),
			byte(value.Address()>>16),
			byte(value.Address()>>8),
			byte(value.Address()),
		).String()
	case dns.DnsRecord_Which_aaaa:
		value, err := record.Aaaa()
		if err != nil {
			return DNSRecordJSON{}, err
		}
		ip := make(net.IP, net.IPv6len)
		binary.BigEndian.PutUint64(ip[:8], value.AddressHigh())
		binary.BigEndian.PutUint64(ip[8:], value.AddressLow())
		out.Address = ip.String()
	case dns.DnsRecord_Which_cname:
		value, err := record.Cname()
		if err != nil {
			return DNSRecordJSON{}, err
		}
		out.Host, err = value.Host()
		if err != nil {
			return DNSRecordJSON{}, err
		}
	case dns.DnsRecord_Which_mx:
		value, err := record.Mx()
		if err != nil {
			return DNSRecordJSON{}, err
		}
		out.Preference = value.Preference()
		out.Exchange, err = value.Exchange()
		if err != nil {
			return DNSRecordJSON{}, err
		}
	case dns.DnsRecord_Which_ns:
		value, err := record.Ns()
		if err != nil {
			return DNSRecordJSON{}, err
		}
		out.Host, err = value.Host()
		if err != nil {
			return DNSRecordJSON{}, err
		}
	case dns.DnsRecord_Which_ptr:
		value, err := record.Ptr()
		if err != nil {
			return DNSRecordJSON{}, err
		}
		out.Host, err = value.Host()
		if err != nil {
			return DNSRecordJSON{}, err
		}
	case dns.DnsRecord_Which_soa:
		value, err := record.Soa()
		if err != nil {
			return DNSRecordJSON{}, err
		}
		out.MName, err = value.Mname()
		if err != nil {
			return DNSRecordJSON{}, err
		}
		out.RName, err = value.Rname()
		if err != nil {
			return DNSRecordJSON{}, err
		}
		out.Serial = value.Serial()
		out.Refresh = value.Refresh()
		out.Retry = value.Retry()
		out.Expire = value.Expire()
		out.Minimum = value.Minimum()
	case dns.DnsRecord_Which_txt:
		value, err := record.Txt()
		if err != nil {
			return DNSRecordJSON{}, err
		}
		list, err := value.Values()
		if err != nil {
			return DNSRecordJSON{}, err
		}
		out.Values = make([]string, 0, list.Len())
		for i := 0; i < list.Len(); i++ {
			item, err := list.At(i)
			if err != nil {
				return DNSRecordJSON{}, err
			}
			out.Values = append(out.Values, item)
		}
	default:
		return DNSRecordJSON{}, fmt.Errorf("unsupported dns union: %v", record.Which())
	}

	return out, nil
}

func decodeTLSChange(bin []byte) (TLSChangeJSON, error) {
	zone, err := ingress.ReadTLSZone(bytes.NewReader(bin))
	if err != nil {
		return TLSChangeJSON{}, err
	}
	name, err := zone.Name()
	if err != nil {
		return TLSChangeJSON{}, err
	}
	policy, err := zone.TlsPolicy()
	if err != nil {
		return TLSChangeJSON{}, err
	}
	sni, err := policy.Sni()
	if err != nil {
		return TLSChangeJSON{}, err
	}
	certPEM, err := policy.CertPem()
	if err != nil {
		return TLSChangeJSON{}, err
	}
	keyPEM, err := policy.KeyPem()
	if err != nil {
		return TLSChangeJSON{}, err
	}

	out := TLSChangeJSON{
		Name:    name,
		SNI:     sni,
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
		Kind:    policy.Kind().String(),
	}
	if policy.HasBackendRef() {
		backend, err := policy.BackendRef()
		if err != nil {
			return TLSChangeJSON{}, err
		}
		out.BackendHostname, err = backend.Hostname()
		if err != nil {
			return TLSChangeJSON{}, err
		}
		out.BackendPort = backend.Port()
	}
	return out, nil
}

func decodeHTTPChange(bin []byte) (HTTPChangeJSON, error) {
	zone, err := ingress.ReadHTTPZone(bytes.NewReader(bin))
	if err != nil {
		return HTTPChangeJSON{}, err
	}
	name, err := zone.Name()
	if err != nil {
		return HTTPChangeJSON{}, err
	}
	list, err := zone.HttpPolicies()
	if err != nil {
		return HTTPChangeJSON{}, err
	}

	out := HTTPChangeJSON{
		Name:     name,
		Policies: make([]HTTPPolicyJSON, 0, list.Len()),
	}
	for i := 0; i < list.Len(); i++ {
		policy, err := decodeHTTPPolicy(list.At(i))
		if err != nil {
			return HTTPChangeJSON{}, fmt.Errorf("decode http policy %d: %w", i, err)
		}
		out.Policies = append(out.Policies, policy)
	}
	return out, nil
}

func decodeHTTPPolicy(policy ingress.HttpPolicy) (HTTPPolicyJSON, error) {
	backend, err := policy.Backend()
	if err != nil {
		return HTTPPolicyJSON{}, err
	}
	out := HTTPPolicyJSON{Backend: backend}

	switch policy.Which() {
	case ingress.HttpPolicy_Which_pathname:
		pathname, err := policy.Pathname()
		if err != nil {
			return HTTPPolicyJSON{}, err
		}
		out.PathnameKind = pathname.Kind().String()
		switch pathname.Which() {
		case ingress.Pathname_Which_exact:
			out.Pathname, err = pathname.Exact()
		case ingress.Pathname_Which_prefix:
			out.Pathname, err = pathname.Prefix()
		case ingress.Pathname_Which_regex:
			out.Pathname, err = pathname.Regex()
		default:
			return HTTPPolicyJSON{}, fmt.Errorf("unsupported pathname union: %v", pathname.Which())
		}
		if err != nil {
			return HTTPPolicyJSON{}, err
		}
	case ingress.HttpPolicy_Which_query:
		query, err := policy.Query()
		if err != nil {
			return HTTPPolicyJSON{}, err
		}
		out.QueryItems, err = decodeKeyValues(query.Items)
		if err != nil {
			return HTTPPolicyJSON{}, err
		}
	case ingress.HttpPolicy_Which_header:
		header, err := policy.Header()
		if err != nil {
			return HTTPPolicyJSON{}, err
		}
		out.HeaderItems, err = decodeKeyValues(header.Items)
		if err != nil {
			return HTTPPolicyJSON{}, err
		}
	default:
		return HTTPPolicyJSON{}, fmt.Errorf("unsupported http policy union: %v", policy.Which())
	}

	return out, nil
}

func decodeKeyValues(load func() (ingress.KeyValue_List, error)) ([]HTTPKVJSON, error) {
	list, err := load()
	if err != nil {
		return nil, err
	}
	items := make([]HTTPKVJSON, 0, list.Len())
	for i := 0; i < list.Len(); i++ {
		entry := list.At(i)
		key, err := entry.Key()
		if err != nil {
			return nil, err
		}
		value, err := entry.Value()
		if err != nil {
			return nil, err
		}
		items = append(items, HTTPKVJSON{Key: key, Value: value})
	}
	return items, nil
}
