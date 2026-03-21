package builder

import dns "aaa/DNS"

type CNAME struct {
	Name string
	TTL  uint32
	Host string
}

func NewCNAME() *CNAME { return &CNAME{TTL: DefaultTTL} }

func (b *CNAME) WithName(name string) *CNAME { b.Name = name; return b }
func (b *CNAME) WithTTL(ttl uint32) *CNAME   { b.TTL = ttl; return b }
func (b *CNAME) WithHost(host string) *CNAME { b.Host = host; return b }
func (b *CNAME) Type() dns.RecordType        { return dns.RecordType_cname }

func (b *CNAME) Build(record dns.DnsRecord) error {
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
	record.SetType(dns.RecordType_cname)
	data, err := record.NewCname()
	if err != nil {
		return err
	}
	return data.SetHost(b.Host)
}
