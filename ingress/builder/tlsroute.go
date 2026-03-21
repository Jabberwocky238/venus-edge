package builder

import (
	"aaa/ingress"
	"fmt"
	"strings"
)

type TLSRouteBuilder struct {
	Name            string
	SNI             string
	CertPEM         string
	KeyPEM          string
	Kind            ingress.TlsPolicy_Kind
	BackendHostname string
	BackendPort     uint16
}

func NewTLSRoute() *TLSRouteBuilder {
	return &TLSRouteBuilder{Kind: ingress.TlsPolicy_Kind_https}
}

func (b *TLSRouteBuilder) WithName(name string) *TLSRouteBuilder {
	b.Name = name
	return b
}

func (b *TLSRouteBuilder) WithSNI(sni string) *TLSRouteBuilder {
	b.SNI = sni
	return b
}

func (b *TLSRouteBuilder) WithCertPEM(certPEM string) *TLSRouteBuilder {
	b.CertPEM = certPEM
	return b
}

func (b *TLSRouteBuilder) WithKeyPEM(keyPEM string) *TLSRouteBuilder {
	b.KeyPEM = keyPEM
	return b
}

func (b *TLSRouteBuilder) WithKind(kind ingress.TlsPolicy_Kind) *TLSRouteBuilder {
	b.Kind = kind
	return b
}

func (b *TLSRouteBuilder) WithBackend(hostname string, port uint16) *TLSRouteBuilder {
	b.BackendHostname = hostname
	b.BackendPort = port
	return b
}

func (b *TLSRouteBuilder) Build(zone ingress.TlsZone) error {
	if err := requireText("name", b.Name); err != nil {
		return err
	}
	if err := requireText("sni", b.SNI); err != nil {
		return err
	}
	if err := zone.SetName(b.Name); err != nil {
		return err
	}
	policy, err := zone.NewTlsPolicy()
	if err != nil {
		return err
	}
	if err := policy.SetSni(b.SNI); err != nil {
		return err
	}
	if err := policy.SetCertPem(b.CertPEM); err != nil {
		return err
	}
	if err := policy.SetKeyPem(b.KeyPEM); err != nil {
		return err
	}
	policy.SetKind(b.Kind)

	switch b.Kind {
	case ingress.TlsPolicy_Kind_tlsPassthrough, ingress.TlsPolicy_Kind_tlsTerminate:
		if err := requireText("backend.hostname", b.BackendHostname); err != nil {
			return err
		}
		if b.BackendPort == 0 {
			return fmt.Errorf("backend.port is required")
		}
		backendRef, err := policy.NewBackendRef()
		if err != nil {
			return err
		}
		if err := backendRef.SetHostname(b.BackendHostname); err != nil {
			return err
		}
		backendRef.SetPort(b.BackendPort)
	case ingress.TlsPolicy_Kind_https:
		if strings.TrimSpace(b.CertPEM) == "" || strings.TrimSpace(b.KeyPEM) == "" {
			return fmt.Errorf("certPem and keyPem are required for https")
		}
	}

	return nil
}

func requireText(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	return nil
}
