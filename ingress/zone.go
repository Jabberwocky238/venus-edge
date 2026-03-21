package ingress

import (
	"aaa/ingress/schema"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	capnp "capnproto.org/go/capnp/v3"
)

const (
	DefaultIngressRoot = ".venus-edge"
	DefaultTLSDir      = "tls"
	DefaultHTTPDir     = "http"
)

type ZoneStore interface {
	OpenTLSZone(zone string) (io.ReadCloser, error)
	OpenHTTPZone(zone string) (io.ReadCloser, error)
}

type FSStore struct {
	Root string
}

type TLSRoute interface {
	Build(zone schema.TlsZone) error
}

type HTTPRoute interface {
	Build(zone schema.HttpZone) error
}

func EnsureZoneDirs(root string) error {
	if err := os.MkdirAll(filepath.Join(root, DefaultTLSDir), 0o755); err != nil {
		return err
	}
	return os.MkdirAll(filepath.Join(root, DefaultHTTPDir), 0o755)
}

func NewFSStore(root string) (FSStore, error) {
	if err := EnsureZoneDirs(root); err != nil {
		return FSStore{}, err
	}
	return FSStore{Root: root}, nil
}

func (s FSStore) OpenTLSZone(zone string) (io.ReadCloser, error) {
	return OpenTLSZoneFile(s.Root, zone)
}

func (s FSStore) OpenHTTPZone(zone string) (io.ReadCloser, error) {
	return OpenHTTPZoneFile(s.Root, zone)
}

func (s FSStore) ReadTLS(zone string) (schema.TlsZone, error) {
	f, err := s.OpenTLSZone(zone)
	if err != nil {
		return schema.TlsZone{}, err
	}
	defer f.Close()
	return ReadTLSZone(f)
}

func (s FSStore) ReadHTTP(zone string) (schema.HttpZone, error) {
	f, err := s.OpenHTTPZone(zone)
	if err != nil {
		return schema.HttpZone{}, err
	}
	defer f.Close()
	return ReadHTTPZone(f)
}

func (s FSStore) WriteTLS(zone string, route TLSRoute) error {
	return WriteTLSZone(TLSZoneFilePath(s.Root, zone), route)
}

func (s FSStore) WriteHTTP(zone string, route HTTPRoute) error {
	return WriteHTTPZone(HTTPZoneFilePath(s.Root, zone), route)
}

func OpenTLSZoneFile(root, zone string) (io.ReadCloser, error) {
	return os.Open(TLSZoneFilePath(root, zone))
}

func OpenHTTPZoneFile(root, zone string) (io.ReadCloser, error) {
	return os.Open(HTTPZoneFilePath(root, zone))
}

func TLSZoneFilePath(root, zone string) string {
	return filepath.Join(root, DefaultTLSDir, sanitizeZoneName(zone)+".bin")
}

func HTTPZoneFilePath(root, zone string) string {
	return filepath.Join(root, DefaultHTTPDir, sanitizeZoneName(zone)+".bin")
}

func CandidateZones(name string) []string {
	trimmed := strings.TrimSuffix(sanitizeZoneName(name), ".")
	if trimmed == "" {
		return nil
	}

	parts := strings.Split(trimmed, ".")
	candidates := []string{trimmed}
	for i := 1; i < len(parts); i++ {
		candidates = append(candidates, "*."+strings.Join(parts[i:], "."))
	}
	return candidates
}

func ReadTLSZone(r io.Reader) (schema.TlsZone, error) {
	msg, err := capnp.NewDecoder(r).Decode()
	if err != nil {
		return schema.TlsZone{}, fmt.Errorf("decode tls zone: %w", err)
	}
	zone, err := schema.ReadRootTlsZone(msg)
	if err != nil {
		return schema.TlsZone{}, fmt.Errorf("read root tls zone: %w", err)
	}
	return zone, nil
}

func ReadHTTPZone(r io.Reader) (schema.HttpZone, error) {
	msg, err := capnp.NewDecoder(r).Decode()
	if err != nil {
		return schema.HttpZone{}, fmt.Errorf("decode http zone: %w", err)
	}
	zone, err := schema.ReadRootHttpZone(msg)
	if err != nil {
		return schema.HttpZone{}, fmt.Errorf("read root http zone: %w", err)
	}
	return zone, nil
}

func WriteTLSZone(path string, route TLSRoute) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create tls dir: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create tls file: %w", err)
	}
	defer f.Close()
	return writeTLSZoneTo(f, route)
}

func WriteHTTPZone(path string, route HTTPRoute) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create http dir: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create http file: %w", err)
	}
	defer f.Close()
	return writeHTTPZoneTo(f, route)
}

func writeTLSZoneTo(w io.Writer, route TLSRoute) error {
	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return fmt.Errorf("new capnp message: %w", err)
	}
	zone, err := NewRootTlsZone(seg)
	if err != nil {
		return fmt.Errorf("new root tls zone: %w", err)
	}
	if err := route.Build(zone); err != nil {
		return fmt.Errorf("build tls zone: %w", err)
	}
	if err := capnp.NewEncoder(w).Encode(msg); err != nil {
		return fmt.Errorf("encode tls zone: %w", err)
	}
	return nil
}

func writeHTTPZoneTo(w io.Writer, route HTTPRoute) error {
	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return fmt.Errorf("new capnp message: %w", err)
	}
	zone, err := NewRootHttpZone(seg)
	if err != nil {
		return fmt.Errorf("new root http zone: %w", err)
	}
	if err := route.Build(zone); err != nil {
		return fmt.Errorf("build http zone: %w", err)
	}
	if err := capnp.NewEncoder(w).Encode(msg); err != nil {
		return fmt.Errorf("encode http zone: %w", err)
	}
	return nil
}

func sanitizeZoneName(zone string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(zone), "."))
}
