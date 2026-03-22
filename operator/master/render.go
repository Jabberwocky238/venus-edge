package master

import (
	"bytes"
	"os"
	"path/filepath"
	"time"

	dns "aaa/DNS"
	ingress "aaa/ingress"
	ingressbuilder "aaa/ingress/builder"
)

func renderTLSRoute(route *ingressbuilder.TLSRouteBuilder) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeTLSZone(&buf, route); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func renderHTTPRoute(route *ingressbuilder.HTTPRouteBuilder) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeHTTPZone(&buf, route); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeDNSRecords(buf *bytes.Buffer, records ...dns.RecordBuilder) error {
	dir, err := os.MkdirTemp("", "master-dns-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "zone.bin")
	if err := dns.Write(path, records...); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	buf.Reset()
	_, _ = buf.Write(data)
	return nil
}

func writeTLSZone(buf *bytes.Buffer, route ingress.TLSRoute) error {
	dir, err := os.MkdirTemp("", "master-tls-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "zone.bin")
	if err := ingress.WriteTLSZone(path, route); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	buf.Reset()
	_, _ = buf.Write(data)
	return nil
}

func writeHTTPZone(buf *bytes.Buffer, route ingress.HTTPRoute) error {
	dir, err := os.MkdirTemp("", "master-http-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "zone.bin")
	if err := ingress.WriteHTTPZone(path, route); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	buf.Reset()
	_, _ = buf.Write(data)
	return nil
}

func envelopeTimestampUnix() int64 {
	return time.Now().Unix()
}
