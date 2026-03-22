package ingress

import (
	"aaa/ingress/schema"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"sync"
)

const (
	tlsLogPrefix = "\033[38;5;183m[TLS]\033[0m"
	tlsLogOK     = "\033[32m"
	tlsLogFail   = "\033[31m"
	tlsLogReset  = "\033[0m"
)

type TLSPolicyFinder interface {
	FindTLSPolicyBySNI(serverName string) (TlsPolicy, error)
}

type TLSConnHandler func(net.Conn) error

type TLSEngineOptions struct {
	Addr   string
	Finder TLSPolicyFinder
	Handle TLSConnHandler
}

type TLSEngine struct {
	addr   string
	finder TLSPolicyFinder
	handle TLSConnHandler

	mu       sync.Mutex
	listener net.Listener
}

var ErrConnHandled = errors.New("connection handled by tls engine")

func NewTLSEngine(opts TLSEngineOptions) *TLSEngine {
	return &TLSEngine{
		addr:   opts.Addr,
		finder: opts.Finder,
		handle: opts.Handle,
	}
}

func (e *TLSEngine) Listen(ctx context.Context) error {
	if e.addr == "" {
		return nil
	}
	if e.finder == nil {
		return fmt.Errorf("tls finder is required")
	}

	listener, err := net.Listen("tcp", e.addr)
	if err != nil {
		return err
	}

	e.mu.Lock()
	if e.listener != nil {
		e.mu.Unlock()
		_ = listener.Close()
		return fmt.Errorf("tls engine already listening")
	}
	e.listener = listener
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		if e.listener == listener {
			e.listener = nil
		}
		e.mu.Unlock()
	}()

	go func() {
		<-ctx.Done()
		_ = e.Stop()
	}()

	for {
		conn, err := e.acceptOne()
		if err != nil {
			if errors.Is(err, ErrConnHandled) {
				continue
			}
			if errors.Is(err, net.ErrClosed) || ctx.Err() != nil {
				return nil
			}
			return err
		}
		if conn == nil {
			continue
		}
		if e.handle == nil {
			_ = conn.Close()
			continue
		}
		go func(c net.Conn) {
			if err := e.handle(c); err != nil {
				_ = c.Close()
			}
		}(conn)
	}
}

func (e *TLSEngine) acceptOne() (net.Conn, error) {
	listener := e.currentListener()
	if listener == nil {
		return nil, net.ErrClosed
	}

	rawConn, err := listener.Accept()
	if err != nil {
		return nil, err
	}

	conn := newPeekConn(rawConn)
	serverName, err := readSNI(conn)
	if err != nil {
		logTLSRoute("", "", err)
		_ = rawConn.Close()
		return nil, ErrConnHandled
	}
	logTLSAccept(serverName)

	policy, err := e.finder.FindTLSPolicyBySNI(serverName)
	if err != nil {
		logTLSRoute(serverName, "", err)
		_ = rawConn.Close()
		return nil, ErrConnHandled
	}

	switch policy.Kind() {
	case TlsPolicy_Kind_tlsPassthrough:
		logTLSRoute(serverName, tlsRouteTarget(policy, "passthrough"), nil)
		go e.proxyPassthrough(conn, policy)
		return nil, ErrConnHandled
	case TlsPolicy_Kind_tlsTerminate:
		logTLSRoute(serverName, tlsRouteTarget(policy, "terminate"), nil)
		go e.proxyTerminated(conn, policy)
		return nil, ErrConnHandled
	case TlsPolicy_Kind_https:
		tlsConn, err := e.terminateTLS(conn, policy)
		if err != nil {
			logTLSRoute(serverName, "https", err)
			_ = rawConn.Close()
			return nil, fmt.Errorf("terminate https for %q: %w", serverName, err)
		}
		logTLSRoute(serverName, "https", nil)
		return tlsConn, nil
	default:
		logTLSRoute(serverName, fmt.Sprintf("unsupported:%v", policy.Kind()), fmt.Errorf("unsupported tls policy kind %v", policy.Kind()))
		_ = rawConn.Close()
		return nil, fmt.Errorf("unsupported tls policy kind %v for %q", policy.Kind(), serverName)
	}
}

func (e *TLSEngine) Close() error {
	return e.Stop()
}

func (e *TLSEngine) Stop() error {
	e.mu.Lock()
	listener := e.listener
	e.listener = nil
	e.mu.Unlock()
	if listener == nil {
		return nil
	}
	return listener.Close()
}

func (e *TLSEngine) Addr() net.Addr {
	listener := e.currentListener()
	if listener == nil {
		return nil
	}
	return listener.Addr()
}

func (e *TLSEngine) currentListener() net.Listener {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.listener
}

type peekConn struct {
	net.Conn

	mu       sync.Mutex
	buf      []byte
	capture  bool
	recorded []byte
}

func newPeekConn(conn net.Conn) *peekConn {
	return &peekConn{Conn: conn, capture: true}
}

func (c *peekConn) Read(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.buf) > 0 {
		n := copy(p, c.buf)
		c.buf = c.buf[n:]
		return n, nil
	}

	n, err := c.Conn.Read(p)
	if c.capture && n > 0 {
		c.recorded = append(c.recorded, p[:n]...)
	}
	return n, err
}

func (c *peekConn) resetReplay() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.capture = false
	c.buf = append(c.buf[:0], c.recorded...)
}

func readSNI(conn *peekConn) (string, error) {
	recordHeader := make([]byte, 5)
	if _, err := io.ReadFull(conn, recordHeader); err != nil {
		return "", fmt.Errorf("read tls record header: %w", err)
	}
	if recordHeader[0] != 22 {
		return "", errors.New("not a tls handshake record")
	}

	recordLen := int(recordHeader[3])<<8 | int(recordHeader[4])
	if recordLen <= 0 {
		return "", errors.New("empty tls record")
	}

	recordBody := make([]byte, recordLen)
	if _, err := io.ReadFull(conn, recordBody); err != nil {
		return "", fmt.Errorf("read tls record body: %w", err)
	}

	conn.resetReplay()
	serverName, err := parseClientHelloSNI(recordBody)
	if err != nil {
		return "", err
	}
	if serverName == "" {
		return "", errors.New("sni not provided")
	}
	return serverName, nil
}

func parseClientHelloSNI(body []byte) (string, error) {
	if len(body) < 4 || body[0] != 1 {
		return "", errors.New("not a client hello")
	}

	helloLen := int(body[1])<<16 | int(body[2])<<8 | int(body[3])
	if helloLen+4 > len(body) {
		return "", errors.New("truncated client hello")
	}

	p := 4
	if p+2+32 > len(body) {
		return "", errors.New("truncated client hello version/random")
	}
	p += 2 + 32

	if p+1 > len(body) {
		return "", errors.New("truncated session id len")
	}
	sessionIDLen := int(body[p])
	p++
	if p+sessionIDLen > len(body) {
		return "", errors.New("truncated session id")
	}
	p += sessionIDLen

	if p+2 > len(body) {
		return "", errors.New("truncated cipher suites len")
	}
	cipherSuitesLen := int(body[p])<<8 | int(body[p+1])
	p += 2
	if p+cipherSuitesLen > len(body) {
		return "", errors.New("truncated cipher suites")
	}
	p += cipherSuitesLen

	if p+1 > len(body) {
		return "", errors.New("truncated compression methods len")
	}
	compressionMethodsLen := int(body[p])
	p++
	if p+compressionMethodsLen > len(body) {
		return "", errors.New("truncated compression methods")
	}
	p += compressionMethodsLen

	if p+2 > len(body) {
		return "", errors.New("truncated extensions len")
	}
	extensionsLen := int(body[p])<<8 | int(body[p+1])
	p += 2
	if p+extensionsLen > len(body) {
		return "", errors.New("truncated extensions")
	}

	extensionsEnd := p + extensionsLen
	for p+4 <= extensionsEnd {
		extType := int(body[p])<<8 | int(body[p+1])
		extLen := int(body[p+2])<<8 | int(body[p+3])
		p += 4
		if p+extLen > extensionsEnd {
			return "", errors.New("truncated extension")
		}
		if extType == 0 {
			return parseServerNameExtension(body[p : p+extLen])
		}
		p += extLen
	}

	return "", nil
}

func parseServerNameExtension(ext []byte) (string, error) {
	if len(ext) < 2 {
		return "", errors.New("truncated server name extension")
	}
	listLen := int(ext[0])<<8 | int(ext[1])
	if listLen+2 > len(ext) {
		return "", errors.New("truncated server name list")
	}
	p := 2
	for p+3 <= len(ext) {
		nameType := ext[p]
		nameLen := int(ext[p+1])<<8 | int(ext[p+2])
		p += 3
		if p+nameLen > len(ext) {
			return "", errors.New("truncated server name")
		}
		if nameType == 0 {
			return string(ext[p : p+nameLen]), nil
		}
		p += nameLen
	}
	return "", nil
}

type StaticTLSPolicyFinder struct {
	policies map[string]TlsPolicy
}

type StoreTLSPolicyFinder struct {
	store FSStore
}

func NewStaticTLSPolicyFinder() *StaticTLSPolicyFinder {
	return &StaticTLSPolicyFinder{
		policies: make(map[string]schema.TlsPolicy),
	}
}

func NewStoreTLSPolicyFinder(store FSStore) *StoreTLSPolicyFinder {
	return &StoreTLSPolicyFinder{store: store}
}

func (f *StaticTLSPolicyFinder) Add(serverName string, policy TlsPolicy) {
	f.policies[serverName] = policy
}

func (f *StaticTLSPolicyFinder) FindTLSPolicyBySNI(serverName string) (TlsPolicy, error) {
	policy, ok := f.policies[serverName]
	if !ok {
		return TlsPolicy{}, fmt.Errorf("tls policy not found for sni %q", serverName)
	}

	return policy, nil
}

func (f *StoreTLSPolicyFinder) FindTLSPolicyBySNI(serverName string) (TlsPolicy, error) {
	for _, zoneName := range CandidateZones(serverName) {
		zone, err := f.store.ReadTLS(zoneName)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return TlsPolicy{}, err
		}
		policy, err := zone.TlsPolicy()
		if err != nil {
			return TlsPolicy{}, fmt.Errorf("read tls policy: %w", err)
		}
		return policy, nil
	}
	return TlsPolicy{}, os.ErrNotExist
}

func (e *TLSEngine) proxyPassthrough(conn net.Conn, policy TlsPolicy) {
	backendConn, err := dialBackend(policy)
	if err != nil {
		_ = conn.Close()
		return
	}
	proxyBidirectional(conn, backendConn)
}

func (e *TLSEngine) proxyTerminated(conn net.Conn, policy TlsPolicy) {
	tlsConn, err := e.terminateTLS(conn, policy)
	if err != nil {
		_ = conn.Close()
		return
	}

	backendConn, err := dialBackend(policy)
	if err != nil {
		_ = tlsConn.Close()
		return
	}
	proxyBidirectional(tlsConn, backendConn)
}

func (e *TLSEngine) terminateTLS(conn net.Conn, policy TlsPolicy) (*tls.Conn, error) {
	crt, err := policy.CertPem()
	if err != nil {
		return nil, fmt.Errorf("read crt: %w", err)
	}
	key, err := policy.KeyPem()
	if err != nil {
		return nil, fmt.Errorf("read key: %w", err)
	}
	if crt == "" || key == "" {
		return nil, errors.New("crt and key are required")
	}

	cert, err := tls.X509KeyPair([]byte(crt), []byte(key))
	if err != nil {
		return nil, fmt.Errorf("load key pair: %w", err)
	}

	tlsConn := tls.Server(conn, &tls.Config{
		Certificates: []tls.Certificate{cert},
	})
	if err := tlsConn.Handshake(); err != nil {
		_ = tlsConn.Close()
		return nil, fmt.Errorf("tls handshake: %w", err)
	}
	return tlsConn, nil
}

func dialBackend(policy TlsPolicy) (net.Conn, error) {
	backend, err := policy.BackendRef()
	if err != nil {
		return nil, fmt.Errorf("read backendRef: %w", err)
	}
	hostname, err := backend.Hostname()
	if err != nil {
		return nil, fmt.Errorf("read backend hostname: %w", err)
	}
	if hostname == "" || backend.Port() == 0 {
		return nil, errors.New("backendRef hostname and port are required")
	}

	addr := net.JoinHostPort(hostname, strconv.FormatUint(uint64(backend.Port()), 10))
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial backend %s: %w", addr, err)
	}
	return conn, nil
}

func proxyBidirectional(left, right net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(left, right)
		if tcp, ok := left.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
			return
		}
		_ = left.Close()
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(right, left)
		if tcp, ok := right.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
			return
		}
		_ = right.Close()
	}()

	wg.Wait()
	_ = left.Close()
	_ = right.Close()
}

func logTLSAccept(serverName string) {
	fmt.Fprintf(os.Stderr, "%s accept sni=%s\n", tlsLogPrefix, serverName)
}

func logTLSRoute(serverName, target string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %sroute%s sni=%s to=%s err=%v\n", tlsLogPrefix, tlsLogFail, tlsLogReset, serverName, target, err)
		return
	}
	fmt.Fprintf(os.Stderr, "%s %sroute%s sni=%s to=%s\n", tlsLogPrefix, tlsLogOK, tlsLogReset, serverName, target)
}

func tlsRouteTarget(policy TlsPolicy, mode string) string {
	backend, err := policy.BackendRef()
	if err != nil {
		return mode
	}
	hostname, err := backend.Hostname()
	if err != nil || hostname == "" || backend.Port() == 0 {
		return mode
	}
	return fmt.Sprintf("%s %s", mode, net.JoinHostPort(hostname, strconv.FormatUint(uint64(backend.Port()), 10)))
}
