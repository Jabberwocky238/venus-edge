package builder

import dns "aaa/DNS"

type NS struct {
	Name string
	TTL  uint32
	Host string
}

func NewNS() *NS { return &NS{TTL: DefaultTTL} }

func (b *NS) WithName(name string) *NS { b.Name = name; return b }
func (b *NS) WithTTL(ttl uint32) *NS   { b.TTL = ttl; return b }
func (b *NS) WithHost(host string) *NS { b.Host = host; return b }
func (b *NS) Type() dns.RecordType     { return dns.RecordType_ns }

func (b *NS) Build(record dns.DnsRecord) error {
	if err := requireText("name", b.Name); err != nil {
		return err
	}
	if err := requireText("host", b.Host); err != nil {
		return err
	}
	if err := record.SetName(b.Name); err != nil {
		return err
	}
	record.SetTtl(b.TTL)
	record.SetType(dns.RecordType_ns)
	data, err := record.NewNs()
	if err != nil {
		return err
	}
	return data.SetHost(b.Host)
}
