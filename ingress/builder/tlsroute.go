package builder

import (
	"aaa/ingress"
	"fmt"
	"strings"
)

type TLSRouteBuilder struct {
	HostName        string
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

func (b *TLSRouteBuilder) WithHostName(hostName string) *TLSRouteBuilder {
	b.HostName = hostName
	return b
}

func (b *TLSRouteBuilder) Use(other *TLSRouteBuilder) *TLSRouteBuilder {
	if other == nil {
		return b
	}
	b.HostName = other.HostName
	b.SNI = other.SNI
	b.CertPEM = other.CertPEM
	b.KeyPEM = other.KeyPEM
	b.Kind = other.Kind
	b.BackendHostname = other.BackendHostname
	b.BackendPort = other.BackendPort
	return b
}

func (b *TLSRouteBuilder) From(zone ingress.TlsZone) error {
	name, err := zone.Hostname()
	if err != nil {
		return err
	}
	policy, err := zone.TlsPolicy()
	if err != nil {
		return err
	}
	sni, err := policy.Sni()
	if err != nil {
		return err
	}
	certPEM, err := policy.CertPem()
	if err != nil {
		return err
	}
	keyPEM, err := policy.KeyPem()
	if err != nil {
		return err
	}

	b.HostName = name
	b.SNI = sni
	b.CertPEM = certPEM
	b.KeyPEM = keyPEM
	b.Kind = policy.Kind()
	b.BackendHostname = ""
	b.BackendPort = 0

	if policy.HasBackendRef() {
		backend, err := policy.BackendRef()
		if err != nil {
			return err
		}
		b.BackendHostname, err = backend.Hostname()
		if err != nil {
			return err
		}
		b.BackendPort = backend.Port()
	}
	return nil
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
	if err := requireText("hostname", b.HostName); err != nil {
		return err
	}
	if err := requireText("sni", b.SNI); err != nil {
		return err
	}
	if err := zone.SetHostname(b.HostName); err != nil {
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

	if strings.TrimSpace(b.BackendHostname) != "" || b.BackendPort != 0 {
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
	}

	switch b.Kind {
	case ingress.TlsPolicy_Kind_tlsPassthrough, ingress.TlsPolicy_Kind_tlsTerminate:
		if err := requireText("backend.hostname", b.BackendHostname); err != nil {
			return err
		}
		if b.BackendPort == 0 {
			return fmt.Errorf("backend.port is required")
		}
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
