package builder

import (
	"encoding/binary"
	"fmt"
	"net"

	dns "aaa/DNS"
)

type AAAA struct {
	Name    string
	TTL     uint32
	Address net.IP
}

func NewAAAA() *AAAA { return &AAAA{TTL: DefaultTTL} }

func (b *AAAA) WithName(name string) *AAAA { b.Name = name; return b }
func (b *AAAA) WithTTL(ttl uint32) *AAAA   { b.TTL = ttl; return b }
func (b *AAAA) WithAddress(ip string) *AAAA {
	b.Address = net.ParseIP(ip)
	return b
}

func (b *AAAA) Type() dns.RecordType { return dns.RecordType_aaaa }

func (b *AAAA) Build(record dns.DnsRecord) error {
	if err := requireText("name", b.Name); err != nil {
		return err
	}
	ip := b.Address.To16()
	if ip == nil || b.Address.To4() != nil {
		return fmt.Errorf("invalid ipv6 address for %q", b.Name)
	}
	if err := record.SetName(b.Name); err != nil {
		return err
	}
	record.SetTtl(b.TTL)
	record.SetType(dns.RecordType_aaaa)
	data, err := record.NewAaaa()
	if err != nil {
		return err
	}
	data.SetAddressHigh(binary.BigEndian.Uint64(ip[:8]))
	data.SetAddressLow(binary.BigEndian.Uint64(ip[8:]))
	return nil
}
