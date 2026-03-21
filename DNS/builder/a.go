package builder

import (
	"fmt"
	"net"

	dns "aaa/DNS"
)

type A struct {
	Name    string
	TTL     uint32
	Address net.IP
}

func NewA() *A { return &A{TTL: DefaultTTL} }

func (b *A) WithName(name string) *A { b.Name = name; return b }
func (b *A) WithTTL(ttl uint32) *A   { b.TTL = ttl; return b }
func (b *A) WithAddress(ip string) *A {
	b.Address = net.ParseIP(ip)
	return b
}

func (b *A) Type() dns.RecordType { return dns.RecordType_a }

func (b *A) Build(record dns.DnsRecord) error {
	if err := requireText("name", b.Name); err != nil {
		return err
	}
	ip4 := b.Address.To4()
	if ip4 == nil {
		return fmt.Errorf("invalid ipv4 address for %q", b.Name)
	}
	if err := record.SetName(b.Name); err != nil {
		return err
	}
	record.SetTtl(b.TTL)
	record.SetType(dns.RecordType_a)
	data, err := record.NewA()
	if err != nil {
		return err
	}
	data.SetAddress(uint32(ip4[0])<<24 | uint32(ip4[1])<<16 | uint32(ip4[2])<<8 | uint32(ip4[3]))
	return nil
}
