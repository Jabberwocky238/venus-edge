package ingress_test

import (
	ingress "aaa/ingress"
	"aaa/ingress/builder"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNewEngineInitializesInternally(t *testing.T) {
	engine := ingress.NewEngine(ingress.EngineOptions{
		Root:       t.TempDir(),
		LogEnabled: 0,
	})
	if engine == nil {
		t.Fatal("expected engine")
	}
}

func TestEngineListenPassthroughRealPort(t *testing.T) {
	root := t.TempDir()

	backendCertPEM, backendKeyPEM := mustSelfSignedPEM(t, "backend.example.com")
	backendTLSCert, err := tls.X509KeyPair([]byte(backendCertPEM), []byte(backendKeyPEM))
	if err != nil {
		t.Fatalf("X509KeyPair() error = %v", err)
	}

	backendLn, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{backendTLSCert},
	})
	if err != nil {
		t.Fatalf("tls.Listen() error = %v", err)
	}
	defer backendLn.Close()

	backendPort := uint16(backendLn.Addr().(*net.TCPAddr).Port)
	backendDone := make(chan struct{})
	go func() {
		defer close(backendDone)
		conn, acceptErr := backendLn.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 4)
		if _, readErr := io.ReadFull(conn, buf); readErr != nil {
			return
		}
		if string(buf) == "ping" {
			_, _ = conn.Write([]byte("pong"))
		}
	}()

	store, err := ingress.NewFSStore(root)
	if err != nil {
		t.Fatalf("NewFSStore() error = %v", err)
	}
	if err := store.WriteTLS("backend.example.com", builder.NewTLSRoute().
		WithName("backend.example.com").
		WithSNI("backend.example.com").
		WithBackend("127.0.0.1", backendPort).
		WithKind(ingress.TlsPolicy_Kind_tlsPassthrough)); err != nil {
		t.Fatalf("WriteTLS() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := ingress.NewEngine(ingress.EngineOptions{Root: root, TLSAddr: "127.0.0.1:0", LogEnabled: 0})
	errCh := make(chan error, 1)
	go func() {
		errCh <- engine.Listen(ctx)
	}()

	addr := waitAddr(t, engine)
	clientConn, err := tls.Dial("tcp", addr, &tls.Config{
		ServerName:         "backend.example.com",
		InsecureSkipVerify: true,
	})
	if err != nil {
		t.Fatalf("tls.Dial() error = %v", err)
	}
	defer clientConn.Close()

	if _, err := clientConn.Write([]byte("ping")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	reply := make([]byte, 4)
	if _, err := io.ReadFull(clientConn, reply); err != nil {
		t.Fatalf("ReadFull() error = %v", err)
	}
	if string(reply) != "pong" {
		t.Fatalf("unexpected reply: %q", string(reply))
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Listen() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("engine did not stop")
	}

	<-backendDone
}

func TestEndToEndFourPortsRouteCorrectly(t *testing.T) {
	root := t.TempDir()

	passthroughBackend := startBackendHTTPSServer(t, "passthrough.example.com", "passthrough-ok")
	defer passthroughBackend.close()

	terminateBackend := startBackendHTTPServer(t, "terminate-ok")
	defer terminateBackend.close()

	httpsBackend := startBackendHTTPServer(t, "https-ok")
	defer httpsBackend.close()

	httpBackend := startBackendHTTPServer(t, "http-ok")
	defer httpBackend.close()

	terminateCertPEM, terminateKeyPEM := mustSelfSignedPEM(t, "terminate.example.com")
	httpsCertPEM, httpsKeyPEM := mustSelfSignedPEM(t, "https.example.com")

	store, err := ingress.NewFSStore(root)
	if err != nil {
		t.Fatalf("NewFSStore() error = %v", err)
	}

	mustWriteTLS(t, store, "passthrough.example.com", builder.NewTLSRoute().
		WithName("passthrough.example.com").
		WithSNI("passthrough.example.com").
		WithBackend("127.0.0.1", passthroughBackend.port).
		WithKind(ingress.TlsPolicy_Kind_tlsPassthrough))

	mustWriteTLS(t, store, "terminate.example.com", builder.NewTLSRoute().
		WithName("terminate.example.com").
		WithSNI("terminate.example.com").
		WithCertPEM(terminateCertPEM).
		WithKeyPEM(terminateKeyPEM).
		WithBackend("127.0.0.1", terminateBackend.port).
		WithKind(ingress.TlsPolicy_Kind_tlsTerminate))

	mustWriteTLS(t, store, "https.example.com", builder.NewTLSRoute().
		WithName("https.example.com").
		WithSNI("https.example.com").
		WithCertPEM(httpsCertPEM).
		WithKeyPEM(httpsKeyPEM).
		WithKind(ingress.TlsPolicy_Kind_https))

	mustWriteHTTP(t, store, "https.example.com", builder.NewHTTPRoute().
		WithName("https.example.com").
		AddPolicy(builder.NewHTTPPolicy().
			WithBackend("http://127.0.0.1:"+itoaPort(httpsBackend.port)).
			WithPrefixPath("/")))

	mustWriteHTTP(t, store, "http.example.com", builder.NewHTTPRoute().
		WithName("http.example.com").
		AddPolicy(builder.NewHTTPPolicy().
			WithBackend("http://127.0.0.1:"+itoaPort(httpBackend.port)).
			WithPrefixPath("/")))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	passthroughFront := startTLSEnginePort(t, ctx, root)
	defer passthroughFront.close()

	terminateFront := startTLSEnginePort(t, ctx, root)
	defer terminateFront.close()

	httpsFront := startIngressEnginePort(t, ctx, root)
	defer httpsFront.close()

	httpFront := startHTTPEnginePort(t, root)
	defer httpFront.close()

	if body := mustHTTPGet(t, "https://passthrough.example.com/", passthroughFront.addr, true); body != "passthrough-ok" {
		t.Fatalf("unexpected passthrough body: %q", body)
	}
	if body := mustHTTPGet(t, "https://terminate.example.com/", terminateFront.addr, true); body != "terminate-ok" {
		t.Fatalf("unexpected terminate body: %q", body)
	}
	if body := mustHTTPGet(t, "https://https.example.com/", httpsFront.addr, true); body != "https-ok" {
		t.Fatalf("unexpected https body: %q", body)
	}
	if body := mustHTTPGet(t, "http://http.example.com/", httpFront.addr, false); body != "http-ok" {
		t.Fatalf("unexpected http body: %q", body)
	}
}

func waitAddr(t *testing.T, engine *ingress.Engine) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if addr := engine.Addr(); addr != nil {
			return addr.String()
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("engine addr not ready")
	return ""
}

type backendServer struct {
	port  uint16
	close func()
}

type frontServer struct {
	addr  string
	close func()
}

func startBackendHTTPServer(t *testing.T, body string) backendServer {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	})}
	go func() { _ = srv.Serve(ln) }()

	return backendServer{
		port: uint16(ln.Addr().(*net.TCPAddr).Port),
		close: func() {
			_ = srv.Close()
			_ = ln.Close()
		},
	}
}

func startBackendHTTPSServer(t *testing.T, host, body string) backendServer {
	t.Helper()

	certPEM, keyPEM := mustSelfSignedPEM(t, host)
	cert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		t.Fatalf("X509KeyPair() error = %v", err)
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatalf("tls.Listen() error = %v", err)
	}

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	})}
	go func() { _ = srv.Serve(ln) }()

	return backendServer{
		port: uint16(ln.Addr().(*net.TCPAddr).Port),
		close: func() {
			_ = srv.Close()
			_ = ln.Close()
		},
	}
}

func startTLSEnginePort(t *testing.T, ctx context.Context, root string) frontServer {
	t.Helper()

	tlsEngine := ingress.NewTLSEngine(ingress.TLSEngineOptions{
		Addr: "127.0.0.1:0",
		Finder: ingress.NewStoreTLSPolicyFinder(func() ingress.FSStore {
			store, err := ingress.NewFSStore(root)
			if err != nil {
				t.Fatalf("NewFSStore() error = %v", err)
			}
			return store
		}()),
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = tlsEngine.Listen(ctx)
	}()

	addr := waitTLSEngineAddr(t, tlsEngine)
	return frontServer{
		addr: addr,
		close: func() {
			_ = tlsEngine.Stop()
			<-done
		},
	}
}

func startIngressEnginePort(t *testing.T, ctx context.Context, root string) frontServer {
	t.Helper()
	engine := ingress.NewEngine(ingress.EngineOptions{Root: root, TLSAddr: "127.0.0.1:0", LogEnabled: 0})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = engine.Listen(ctx)
	}()
	addr := waitAddr(t, engine)
	return frontServer{
		addr: addr,
		close: func() {
			_ = engine.Stop()
			<-done
		},
	}
}

func startHTTPEnginePort(t *testing.T, root string) frontServer {
	t.Helper()

	httpEngine := ingress.NewHTTPEngine(ingress.HTTPEngineOptions{Root: root, Addr: "127.0.0.1:0"})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = httpEngine.Listen(context.Background())
	}()
	addr := waitHTTPEngineAddr(t, httpEngine)

	return frontServer{
		addr: addr,
		close: func() {
			_ = httpEngine.Stop()
			<-done
		},
	}
}

func waitTLSEngineAddr(t *testing.T, engine *ingress.TLSEngine) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if addr := engine.Addr(); addr != nil {
			return addr.String()
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("tls engine addr not ready")
	return ""
}

func waitHTTPEngineAddr(t *testing.T, engine *ingress.HTTPEngine) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if addr := engine.Addr(); addr != nil {
			return addr.String()
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("http engine addr not ready")
	return ""
}

func mustHTTPGet(t *testing.T, rawURL, addr string, useTLS bool) string {
	t.Helper()

	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	}
	if useTLS {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	client := &http.Client{Transport: tr, Timeout: 3 * time.Second}

	resp, err := client.Get(rawURL)
	if err != nil {
		t.Fatalf("Get(%q) error = %v", rawURL, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	return string(data)
}

func mustWriteTLS(t *testing.T, store ingress.FSStore, zone string, route ingress.TLSRoute) {
	t.Helper()
	if err := store.WriteTLS(zone, route); err != nil {
		t.Fatalf("WriteTLS(%q) error = %v", zone, err)
	}
}

func mustWriteHTTP(t *testing.T, store ingress.FSStore, zone string, route ingress.HTTPRoute) {
	t.Helper()
	if err := store.WriteHTTP(zone, route); err != nil {
		t.Fatalf("WriteHTTP(%q) error = %v", zone, err)
	}
}

func itoaPort(port uint16) string {
	return strconv.FormatUint(uint64(port), 10)
}

func mustSelfSignedPEM(t *testing.T, commonName string) (string, string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{commonName},
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}

	var certPEM strings.Builder
	if err := pem.Encode(&certPEM, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatalf("pem.Encode(cert) error = %v", err)
	}

	var keyPEM strings.Builder
	if err := pem.Encode(&keyPEM, &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}); err != nil {
		t.Fatalf("pem.Encode(key) error = %v", err)
	}

	return certPEM.String(), keyPEM.String()
}
