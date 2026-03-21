package builder

import dns "aaa/DNS"

type MX struct {
	Name       string
	TTL        uint32
	Preference uint16
	Exchange   string
}

func NewMX() *MX { return &MX{TTL: DefaultTTL} }

func (b *MX) WithName(name string) *MX         { b.Name = name; return b }
func (b *MX) WithTTL(ttl uint32) *MX           { b.TTL = ttl; return b }
func (b *MX) WithPreference(v uint16) *MX      { b.Preference = v; return b }
func (b *MX) WithExchange(exchange string) *MX { b.Exchange = exchange; return b }
func (b *MX) Type() dns.RecordType             { return dns.RecordType_mx }
func (b *MX) Build(record dns.DnsRecord) error {
	if err := requireText("name", b.Name); err != nil {
		return err
	}
	if err := requireText("exchange", b.Exchange); err != nil {
		return err
	}
	if err := record.SetName(b.Name); err != nil {
		return err
	}
	record.SetTtl(b.TTL)
	record.SetType(dns.RecordType_mx)
	data, err := record.NewMx()
	if err != nil {
		return err
	}
	data.SetPreference(b.Preference)
	return data.SetExchange(b.Exchange)
}
