package builder

import dns "aaa/DNS"

type PTR struct {
	Name string
	TTL  uint32
	Host string
}

func NewPTR() *PTR { return &PTR{TTL: DefaultTTL} }

func (b *PTR) WithName(name string) *PTR { b.Name = name; return b }
func (b *PTR) WithTTL(ttl uint32) *PTR   { b.TTL = ttl; return b }
func (b *PTR) WithHost(host string) *PTR { b.Host = host; return b }
func (b *PTR) Type() dns.RecordType      { return dns.RecordType_ptr }

func (b *PTR) Build(record dns.DnsRecord) error {
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
	record.SetType(dns.RecordType_ptr)
	data, err := record.NewPtr()
	if err != nil {
		return err
	}
	return data.SetHost(b.Host)
}
