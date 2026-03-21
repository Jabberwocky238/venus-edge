package builder

import dns "aaa/DNS"

type TXT struct {
	Name   string
	TTL    uint32
	Values []string
}

func NewTXT() *TXT { return &TXT{TTL: DefaultTTL} }

func (b *TXT) WithName(name string) *TXT { b.Name = name; return b }
func (b *TXT) WithTTL(ttl uint32) *TXT   { b.TTL = ttl; return b }
func (b *TXT) WithValues(values ...string) *TXT {
	b.Values = append(b.Values[:0], values...)
	return b
}
func (b *TXT) Type() dns.RecordType { return dns.RecordType_txt }

func (b *TXT) Build(record dns.DnsRecord) error {
	if err := requireText("name", b.Name); err != nil {
		return err
	}
	if err := requireTexts("values", b.Values); err != nil {
		return err
	}
	if err := record.SetName(b.Name); err != nil {
		return err
	}
	record.SetTtl(b.TTL)
	record.SetType(dns.RecordType_txt)
	data, err := record.NewTxt()
	if err != nil {
		return err
	}
	list, err := data.NewValues(int32(len(b.Values)))
	if err != nil {
		return err
	}
	for i, v := range b.Values {
		if err := list.Set(i, v); err != nil {
			return err
		}
	}
	return nil
}
