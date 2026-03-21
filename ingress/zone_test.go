package ingress_test

import (
	ingress "aaa/ingress"
	"aaa/ingress/builder"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNewFSStoreCreatesDirs(t *testing.T) {
	root := t.TempDir()

	store, err := ingress.NewFSStore(root)
	if err != nil {
		t.Fatalf("NewFSStore() error = %v", err)
	}
	if store.Root != root {
		t.Fatalf("unexpected root: %q", store.Root)
	}

	for _, path := range []string{
		filepath.Join(root, ingress.DefaultTLSDir),
		filepath.Join(root, ingress.DefaultHTTPDir),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat(%q) error = %v", path, err)
		}
		if !info.IsDir() {
			t.Fatalf("%q is not a directory", path)
		}
	}
}

func TestFSStoreWriteAndReadTLS(t *testing.T) {
	root := t.TempDir()
	store, err := ingress.NewFSStore(root)
	if err != nil {
		t.Fatalf("NewFSStore() error = %v", err)
	}

	err = store.WriteTLS("example.com", builder.NewTLSRoute().
		WithName("example.com").
		WithSNI("example.com").
		WithCertPEM("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----").
		WithKeyPEM("-----BEGIN PRIVATE KEY-----\nMIIB\n-----END PRIVATE KEY-----").
		WithBackend("127.0.0.1", 9443).
		WithKind(ingress.TlsPolicy_Kind_https))
	if err != nil {
		t.Fatalf("WriteTLS() error = %v", err)
	}

	zone, err := store.ReadTLS("example.com")
	if err != nil {
		t.Fatalf("ReadTLS() error = %v", err)
	}
	name, err := zone.Name()
	if err != nil || name != "example.com" {
		t.Fatalf("unexpected name: %q err=%v", name, err)
	}
	policy, err := zone.TlsPolicy()
	if err != nil {
		t.Fatalf("TlsPolicy() error = %v", err)
	}
	if got := policy.Kind(); got != ingress.TlsPolicy_Kind_https {
		t.Fatalf("unexpected kind: %v", got)
	}
	if policy.HasBackendRef() {
		t.Fatal("https policy should not require backendRef")
	}
}

func TestFSStoreWriteAndReadTLSPassthroughBackendRef(t *testing.T) {
	root := t.TempDir()
	store, err := ingress.NewFSStore(root)
	if err != nil {
		t.Fatalf("NewFSStore() error = %v", err)
	}

	err = store.WriteTLS("tcp.example.com", builder.NewTLSRoute().
		WithName("tcp.example.com").
		WithSNI("tcp.example.com").
		WithBackend("127.0.0.1", 9443).
		WithKind(ingress.TlsPolicy_Kind_tlsPassthrough))
	if err != nil {
		t.Fatalf("WriteTLS() error = %v", err)
	}

	zone, err := store.ReadTLS("tcp.example.com")
	if err != nil {
		t.Fatalf("ReadTLS() error = %v", err)
	}
	policy, err := zone.TlsPolicy()
	if err != nil {
		t.Fatalf("TlsPolicy() error = %v", err)
	}
	backendRef, err := policy.BackendRef()
	if err != nil {
		t.Fatalf("BackendRef() error = %v", err)
	}
	hostname, err := backendRef.Hostname()
	if err != nil {
		t.Fatalf("Hostname() error = %v", err)
	}
	if hostname != "127.0.0.1" || backendRef.Port() != 9443 {
		t.Fatalf("unexpected backendRef: %s:%d", hostname, backendRef.Port())
	}
}

func TestFSStoreWriteAndReadHTTP(t *testing.T) {
	root := t.TempDir()
	store, err := ingress.NewFSStore(root)
	if err != nil {
		t.Fatalf("NewFSStore() error = %v", err)
	}

	err = store.WriteHTTP("example.com", builder.NewHTTPRoute().
		WithName("example.com").
		AddPolicy(builder.NewHTTPPolicy().
			WithBackend("http://127.0.0.1:8080").
			WithPrefixPath("/api")).
		AddPolicy(builder.NewHTTPPolicy().
			WithBackend("https://upstream.internal").
			WithHeader("x-env", "prod")))
	if err != nil {
		t.Fatalf("WriteHTTP() error = %v", err)
	}

	zone, err := store.ReadHTTP("example.com")
	if err != nil {
		t.Fatalf("ReadHTTP() error = %v", err)
	}
	name, err := zone.Name()
	if err != nil || name != "example.com" {
		t.Fatalf("unexpected name: %q err=%v", name, err)
	}
	policies, err := zone.HttpPolicies()
	if err != nil {
		t.Fatalf("HttpPolicies() error = %v", err)
	}
	if policies.Len() != 2 {
		t.Fatalf("expected 2 policies, got %d", policies.Len())
	}
	backend, err := policies.At(0).Backend()
	if err != nil || backend != "http://127.0.0.1:8080" {
		t.Fatalf("unexpected backend: %q err=%v", backend, err)
	}
}

func TestReadMissingZoneReturnsNotExist(t *testing.T) {
	root := t.TempDir()
	store, err := ingress.NewFSStore(root)
	if err != nil {
		t.Fatalf("NewFSStore() error = %v", err)
	}

	_, err = store.ReadTLS("missing.example.com")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}

	_, err = store.ReadHTTP("missing.example.com")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}

func TestStoreTLSPolicyFinderMatchesWildcard(t *testing.T) {
	root := t.TempDir()
	store, err := ingress.NewFSStore(root)
	if err != nil {
		t.Fatalf("NewFSStore() error = %v", err)
	}

	err = store.WriteTLS("*.example.com", builder.NewTLSRoute().
		WithName("*.example.com").
		WithSNI("*.example.com").
		WithBackend("127.0.0.1", 9443).
		WithKind(ingress.TlsPolicy_Kind_tlsPassthrough))
	if err != nil {
		t.Fatalf("WriteTLS() error = %v", err)
	}

	finder := ingress.NewStoreTLSPolicyFinder(store)
	policy, err := finder.FindTLSPolicyBySNI("api.example.com")
	if err != nil {
		t.Fatalf("FindTLSPolicyBySNI() error = %v", err)
	}
	if got := policy.Kind(); got != ingress.TlsPolicy_Kind_tlsPassthrough {
		t.Fatalf("unexpected kind: %v", got)
	}
}
