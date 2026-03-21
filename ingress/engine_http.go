package ingress

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
)

const (
	httpLogPrefix = "\033[38;5;229m[HTTP]\033[0m"
	httpLogOK     = "\033[32m"
	httpLogFail   = "\033[31m"
	httpLogReset  = "\033[0m"
)

type matchedHTTPBackend struct {
	backend     string
	prunePrefix string
}

type HTTPEngineOptions struct {
	Root string
	Addr string
}

type HTTPEngine struct {
	store  FSStore
	server *http.Server
	addr   string

	mu       sync.Mutex
	listener net.Listener
}

func NewHTTPEngine(opts HTTPEngineOptions) *HTTPEngine {
	root := opts.Root
	if root == "" {
		root = DefaultIngressRoot
	}

	store, err := NewFSStore(root)
	if err != nil {
		panic(err)
	}

	engine := &HTTPEngine{store: store, addr: opts.Addr}
	engine.server = &http.Server{Handler: engine.Handler()}
	return engine
}

func (e *HTTPEngine) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logHTTPRequest(r)
		recorder := &httpLogResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		defer logHTTPResponse(r, recorder.statusCode, http.StatusText(recorder.statusCode))

		match, err := e.lookupBackend(r)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				http.NotFound(recorder, r)
				return
			}
			http.Error(recorder, err.Error(), http.StatusBadGateway)
			return
		}

		target, err := url.Parse(match.backend)
		if err != nil {
			http.Error(recorder, fmt.Sprintf("invalid backend %q: %v", match.backend, err), http.StatusBadGateway)
			return
		}

		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.Rewrite = func(req *httputil.ProxyRequest) {
			rewrittenPath := req.In.URL.Path
			rewrittenRawPath := req.In.URL.RawPath
			if match.prunePrefix != "" {
				rewrittenPath = pruneRequestPath(rewrittenPath, match.prunePrefix)
				rewrittenRawPath = pruneRequestPath(rewrittenRawPath, match.prunePrefix)
			}

			req.SetURL(target)
			req.Out.URL.Path = joinBackendPath(target.Path, rewrittenPath)
			req.Out.URL.RawPath = joinBackendPath(target.RawPath, rewrittenRawPath)
		}
		proxy.Director = nil
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
		proxy.ServeHTTP(recorder, r)
	})
}

func (e *HTTPEngine) Listen(ctx context.Context) error {
	if e.addr == "" {
		return nil
	}

	listener, err := net.Listen("tcp", e.addr)
	if err != nil {
		return err
	}

	e.mu.Lock()
	if e.listener != nil {
		e.mu.Unlock()
		_ = listener.Close()
		return fmt.Errorf("http engine already listening")
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

	err = e.server.Serve(listener)
	if err == nil || errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) || ctx.Err() != nil {
		return nil
	}
	return err
}

func (e *HTTPEngine) lookupBackend(r *http.Request) (matchedHTTPBackend, error) {
	zone, err := e.findZone(r.Host)
	if err != nil {
		return matchedHTTPBackend{}, err
	}

	policies, err := zone.HttpPolicies()
	if err != nil {
		return matchedHTTPBackend{}, fmt.Errorf("read http policies: %w", err)
	}

	for i := 0; i < policies.Len(); i++ {
		policy := policies.At(i)
		prunePrefix, match, err := matchPolicy(policy, r)
		if err != nil {
			return matchedHTTPBackend{}, fmt.Errorf("match policy %d: %w", i, err)
		}
		if !match {
			continue
		}
		backend, err := policy.Backend()
		if err != nil {
			return matchedHTTPBackend{}, fmt.Errorf("read backend: %w", err)
		}
		return matchedHTTPBackend{backend: backend, prunePrefix: prunePrefix}, nil
	}

	return matchedHTTPBackend{}, os.ErrNotExist
}

func (e *HTTPEngine) findZone(host string) (HttpZone, error) {
	for _, zoneName := range CandidateZones(host) {
		zone, err := e.store.ReadHTTP(zoneName)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return HttpZone{}, err
		}
		return zone, nil
	}
	return HttpZone{}, os.ErrNotExist
}

func matchPolicy(policy HttpPolicy, r *http.Request) (string, bool, error) {
	switch policy.Which() {
	case HttpPolicy_Which_pathname:
		pathname, err := policy.Pathname()
		if err != nil {
			return "", false, err
		}
		return matchPathname(pathname, r.URL.Path)
	case HttpPolicy_Which_query:
		query, err := policy.Query()
		if err != nil {
			return "", false, err
		}
		ok, err := matchKeyValues(query.Items, r.URL.Query().Get)
		return "", ok, err
	case HttpPolicy_Which_header:
		header, err := policy.Header()
		if err != nil {
			return "", false, err
		}
		ok, err := matchKeyValues(header.Items, r.Header.Get)
		return "", ok, err
	default:
		return "", false, fmt.Errorf("unsupported policy selector %v", policy.Which())
	}
}

type keyValueGetter func() (KeyValue_List, error)
type valueLookup func(string) string

func matchKeyValues(getter keyValueGetter, lookup valueLookup) (bool, error) {
	items, err := getter()
	if err != nil {
		return false, err
	}
	for i := 0; i < items.Len(); i++ {
		item := items.At(i)
		key, err := item.Key()
		if err != nil {
			return false, err
		}
		value, err := item.Value()
		if err != nil {
			return false, err
		}
		if lookup(key) != value {
			return false, nil
		}
	}
	return true, nil
}

func matchPathname(pathname Pathname, requestPath string) (string, bool, error) {
	switch pathname.Which() {
	case Pathname_Which_exact:
		value, err := pathname.Exact()
		return "", requestPath == value, err
	case Pathname_Which_prefix:
		value, err := pathname.Prefix()
		if err != nil {
			return "", false, err
		}
		return value, len(requestPath) >= len(value) && requestPath[:len(value)] == value, nil
	case Pathname_Which_regex:
		value, err := pathname.Regex()
		if err != nil {
			return "", false, err
		}
		ok, err := regexp.MatchString(value, requestPath)
		return "", ok, err
	default:
		return "", false, fmt.Errorf("unsupported pathname selector %v", pathname.Which())
	}
}

func pruneRequestPath(path, prefix string) string {
	if path == "" || prefix == "" || len(path) < len(prefix) || path[:len(prefix)] != prefix {
		return path
	}
	pruned := path[len(prefix):]
	if pruned == "" {
		return "/"
	}
	if pruned[0] != '/' {
		return "/" + pruned
	}
	return pruned
}

func joinBackendPath(basePath, requestPath string) string {
	switch {
	case basePath == "":
		if requestPath == "" {
			return "/"
		}
		return requestPath
	case requestPath == "":
		if basePath == "" {
			return "/"
		}
		return basePath
	case strings.HasSuffix(basePath, "/") && strings.HasPrefix(requestPath, "/"):
		return basePath + requestPath[1:]
	case !strings.HasSuffix(basePath, "/") && !strings.HasPrefix(requestPath, "/"):
		return basePath + "/" + requestPath
	default:
		return basePath + requestPath
	}
}

func (e *HTTPEngine) ServeConn(conn net.Conn) error {
	ln := &singleConnListener{conn: conn}
	err := e.server.Serve(ln)
	if err == nil || errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

func (e *HTTPEngine) Stop() error {
	e.mu.Lock()
	listener := e.listener
	e.listener = nil
	e.mu.Unlock()
	if listener == nil {
		return nil
	}
	_ = e.server.Close()
	return listener.Close()
}

func (e *HTTPEngine) Addr() net.Addr {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.listener == nil {
		return nil
	}
	return e.listener.Addr()
}

type singleConnListener struct {
	conn     net.Conn
	accepted bool
}

type httpLogResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *httpLogResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (l *singleConnListener) Accept() (net.Conn, error) {
	if l.accepted || l.conn == nil {
		return nil, net.ErrClosed
	}
	l.accepted = true
	return l.conn, nil
}

func (l *singleConnListener) Close() error {
	return nil
}

func (l *singleConnListener) Addr() net.Addr {
	if l.conn != nil {
		return l.conn.LocalAddr()
	}
	return &net.TCPAddr{}
}

func logHTTPRequest(r *http.Request) {
	if r == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "%s request %s %s\n", httpLogPrefix, r.Method, fullRequestPath(r))
}

func logHTTPResponse(r *http.Request, statusCode int, statusText string) {
	if r == nil {
		return
	}
	color := httpLogOK
	if statusCode >= http.StatusBadRequest {
		color = httpLogFail
	}
	fmt.Fprintf(
		os.Stderr,
		"%s response %s %s %s[%d] %s%s\n",
		httpLogPrefix,
		r.Method,
		fullRequestPath(r),
		color,
		statusCode,
		statusText,
		httpLogReset,
	)
}

func fullRequestPath(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	if r.URL.RawQuery == "" {
		return r.URL.Path
	}
	return r.URL.Path + "?" + r.URL.RawQuery
}
