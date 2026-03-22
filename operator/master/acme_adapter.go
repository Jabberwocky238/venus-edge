package master

import (
	acme "aaa/operator/master/acme"
	"context"
)

func (m *Master) Root() string {
	if m == nil || m.root == "" {
		return defaultMasterRoot
	}
	return m.root
}

func (m *Master) ReadHTTP(ctx context.Context, hostname string) (acme.HTTPChange, error) {
	change, err := m.ReadHTTPJSON(ctx, hostname)
	if err != nil {
		return acme.HTTPChange{}, err
	}
	return toACMEHTTPChange(change), nil
}

func (m *Master) PublishHTTPChange(ctx context.Context, hostname string, change acme.HTTPChange) error {
	payload, err := marshalACMEHTTPChange(change)
	if err != nil {
		return err
	}
	_, err = m.PublishHTTPJSON(ctx, hostname, payload)
	return err
}

func (m *Master) ReadTLS(ctx context.Context, hostname string) (acme.TLSChange, error) {
	change, err := m.ReadTLSJSON(ctx, hostname)
	if err != nil {
		return acme.TLSChange{}, err
	}
	return acme.TLSChange{
		Name:            change.Name,
		SNI:             change.SNI,
		Kind:            change.Kind,
		CertPEM:         change.CertPEM,
		KeyPEM:          change.KeyPEM,
		BackendHostname: change.BackendHostname,
		BackendPort:     change.BackendPort,
	}, nil
}

func (m *Master) PublishTLSChange(ctx context.Context, hostname string, change acme.TLSChange) error {
	bin, err := renderTLSChange(TLSChangeJSON{
		Name:            change.Name,
		SNI:             change.SNI,
		CertPEM:         change.CertPEM,
		KeyPEM:          change.KeyPEM,
		Kind:            change.Kind,
		BackendHostname: change.BackendHostname,
		BackendPort:     change.BackendPort,
	})
	if err != nil {
		return err
	}
	_, err = m.PublishTLS(ctx, hostname, bin)
	return err
}
