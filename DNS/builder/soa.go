package builder

import dns "aaa/DNS"

type SOA struct {
	Name    string
	TTL     uint32
	MName   string
	RName   string
	Serial  uint32
	Refresh uint32
	Retry   uint32
	Expire  uint32
	Minimum uint32
}

func NewSOA() *SOA {
	return &SOA{
		TTL:     DefaultTTL,
		Serial:  1,
		Refresh: 3600,
		Retry:   600,
		Expire:  1209600,
		Minimum: 300,
	}
}

func (b *SOA) WithName(name string) *SOA { b.Name = name; return b }
func (b *SOA) WithTTL(ttl uint32) *SOA   { b.TTL = ttl; return b }
func (b *SOA) WithMName(v string) *SOA   { b.MName = v; return b }
func (b *SOA) WithRName(v string) *SOA   { b.RName = v; return b }
func (b *SOA) WithSerial(v uint32) *SOA  { b.Serial = v; return b }
func (b *SOA) WithRefresh(v uint32) *SOA { b.Refresh = v; return b }
func (b *SOA) WithRetry(v uint32) *SOA   { b.Retry = v; return b }
func (b *SOA) WithExpire(v uint32) *SOA  { b.Expire = v; return b }
func (b *SOA) WithMinimum(v uint32) *SOA { b.Minimum = v; return b }
func (b *SOA) Type() dns.RecordType      { return dns.RecordType_soa }

func (b *SOA) Build(record dns.DnsRecord) error {
	if err := requireText("name", b.Name); err != nil {
		return err
	}
	if err := requireText("mname", b.MName); err != nil {
		return err
	}
	if err := requireText("rname", b.RName); err != nil {
		return err
	}
	if err := record.SetName(b.Name); err != nil {
		return err
	}
	record.SetTtl(b.TTL)
	record.SetType(dns.RecordType_soa)
	data, err := record.NewSoa()
	if err != nil {
		return err
	}
	if err := data.SetMname(b.MName); err != nil {
		return err
	}
	if err := data.SetRname(b.RName); err != nil {
		return err
	}
	data.SetSerial(b.Serial)
	data.SetRefresh(b.Refresh)
	data.SetRetry(b.Retry)
	data.SetExpire(b.Expire)
	data.SetMinimum(b.Minimum)
	return nil
}
