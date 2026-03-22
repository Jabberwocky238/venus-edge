package master

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	dns "aaa/DNS"
	dnsbuilder "aaa/DNS/builder"
	ingress "aaa/ingress"
	ingressbuilder "aaa/ingress/builder"
	"aaa/operator/replication"
)

type ManageServer struct {
	master  *Master
	hub     *Hub
	webRoot string
	mux     *http.ServeMux
}

type manageOverview struct {
	NowUnix     int64                `json:"now_unix"`
	Subscribers []SubscriberSnapshot `json:"subscribers"`
}

type manageDNSPayload struct {
	Records []manageDNSRecord `json:"records"`
}

type manageDNSRecord struct {
	Type       string   `json:"type"`
	Name       string   `json:"name"`
	TTL        uint32   `json:"ttl,omitempty"`
	Address    string   `json:"address,omitempty"`
	Host       string   `json:"host,omitempty"`
	Values     []string `json:"values,omitempty"`
	Preference uint16   `json:"preference,omitempty"`
	Exchange   string   `json:"exchange,omitempty"`
	MName      string   `json:"mname,omitempty"`
	RName      string   `json:"rname,omitempty"`
	Serial     uint32   `json:"serial,omitempty"`
	Refresh    uint32   `json:"refresh,omitempty"`
	Retry      uint32   `json:"retry,omitempty"`
	Expire     uint32   `json:"expire,omitempty"`
	Minimum    uint32   `json:"minimum,omitempty"`
}

type manageTLSPayload struct {
	Name            string `json:"name"`
	SNI             string `json:"sni"`
	CertPEM         string `json:"cert_pem,omitempty"`
	KeyPEM          string `json:"key_pem,omitempty"`
	Kind            string `json:"kind,omitempty"`
	BackendHostname string `json:"backend_hostname,omitempty"`
	BackendPort     uint16 `json:"backend_port,omitempty"`
}

type manageHTTPPayload struct {
	Name     string             `json:"name"`
	Policies []manageHTTPPolicy `json:"policies"`
}

type manageHTTPPolicy struct {
	Backend        string               `json:"backend"`
	PathnameKind   string               `json:"pathname_kind,omitempty"`
	Pathname       string               `json:"pathname,omitempty"`
	QueryItems     []manageHTTPKeyValue `json:"query_items,omitempty"`
	HeaderItems    []manageHTTPKeyValue `json:"header_items,omitempty"`
	FixContent     string               `json:"fix_content,omitempty"`
	AllowRawAccess bool                 `json:"allow_raw_access,omitempty"`
}

type manageHTTPKeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func NewManageServer(m *Master, hub *Hub, webRoot string) (*ManageServer, error) {
	if m == nil {
		return nil, fmt.Errorf("master is required")
	}
	if hub == nil {
		return nil, fmt.Errorf("hub is required")
	}
	if webRoot == "" {
		webRoot = filepath.Join("operator", "web", "dist")
	}

	s := &ManageServer{
		master:  m,
		hub:     hub,
		webRoot: webRoot,
		mux:     http.NewServeMux(),
	}
	s.routes()
	return s, nil
}

func (s *ManageServer) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			logManageRequest(r)
			recorder := &manageLogResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			defer logManageResponse(r, recorder.statusCode, http.StatusText(recorder.statusCode))
			w = recorder
		}
		setCORSHeaders(w, r)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		s.mux.ServeHTTP(w, r)
	})
}

func (s *ManageServer) ListenAndServe(addr string) error {
	if addr == "" {
		addr = ":8080"
	}
	server := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return server.ListenAndServe()
}

func (s *ManageServer) routes() {
	s.mux.HandleFunc("/api/healthz", s.handleHealthz)
	s.mux.HandleFunc("/api/master/overview", s.handleOverview)
	s.mux.HandleFunc("/api/master/dns", s.handleDNS)
	s.mux.HandleFunc("/api/master/tls", s.handleTLS)
	s.mux.HandleFunc("/api/master/http", s.handleHTTP)
	s.mux.Handle("/", s.staticHandler())
}

func (s *ManageServer) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{
		"ok":   true,
		"now":  time.Now().Unix(),
		"root": s.webRoot,
	})
}

func (s *ManageServer) handleOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, manageOverview{
		NowUnix:     time.Now().Unix(),
		Subscribers: s.hub.Snapshot(),
	})
}

func (s *ManageServer) handleDNS(w http.ResponseWriter, r *http.Request) {
	s.handleResource(
		w,
		r,
		func(ctx context.Context, hostname string) (any, error) {
			bin, err := s.master.readObject(ctx, replication.EventType_EVENT_TYPE_DNS, hostname)
			if err != nil {
				return nil, err
			}
			return dnsPayloadFromBin(bin)
		},
		func(ctx context.Context, hostname string, payload []byte) (any, error) {
			var input manageDNSPayload
			if err := json.Unmarshal(payload, &input); err != nil {
				return nil, fmt.Errorf("decode dns builder json: %w", err)
			}
			bin, err := dnsPayloadToBin(input)
			if err != nil {
				return nil, err
			}
			return s.master.PublishDNS(ctx, hostname, bin)
		},
	)
}

func (s *ManageServer) handleTLS(w http.ResponseWriter, r *http.Request) {
	s.handleResource(
		w,
		r,
		func(ctx context.Context, hostname string) (any, error) {
			route, err := s.master.ReadTLSRoute(ctx, hostname)
			if err != nil {
				return nil, err
			}
			return tlsPayloadFromBuilder(route), nil
		},
		func(ctx context.Context, hostname string, payload []byte) (any, error) {
			var input manageTLSPayload
			if err := json.Unmarshal(payload, &input); err != nil {
				return nil, fmt.Errorf("decode tls builder json: %w", err)
			}
			route, err := tlsRouteFromPayload(hostname, input)
			if err != nil {
				return nil, err
			}
			resp, err := s.master.PublishTLSRouteWithResponse(ctx, hostname, route)
			if err != nil {
				return nil, err
			}
			return resp, nil
		},
	)
}

func (s *ManageServer) handleHTTP(w http.ResponseWriter, r *http.Request) {
	s.handleResource(
		w,
		r,
		func(ctx context.Context, hostname string) (any, error) {
			route, err := s.master.ReadHTTPRoute(ctx, hostname)
			if err != nil {
				return nil, err
			}
			return httpPayloadFromBuilder(route), nil
		},
		func(ctx context.Context, hostname string, payload []byte) (any, error) {
			var input manageHTTPPayload
			if err := json.Unmarshal(payload, &input); err != nil {
				return nil, fmt.Errorf("decode http builder json: %w", err)
			}
			route, err := httpRouteFromPayload(hostname, input)
			if err != nil {
				return nil, err
			}
			resp, err := s.master.PublishHTTPRouteWithACME(ctx, hostname, route)
			if err != nil {
				return nil, err
			}
			return resp, nil
		},
	)
}

func (s *ManageServer) handleResource(
	w http.ResponseWriter,
	r *http.Request,
	read func(context.Context, string) (any, error),
	write func(context.Context, string, []byte) (any, error),
) {
	hostname := strings.TrimSpace(r.URL.Query().Get("hostname"))
	if hostname == "" {
		http.Error(w, "hostname is required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		result, err := read(r.Context(), hostname)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, result)
	case http.MethodPut:
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read request body failed", http.StatusBadRequest)
			return
		}
		result, err := write(r.Context(), hostname, payload)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, result)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *ManageServer) staticHandler() http.Handler {
	distIndex := filepath.Join(s.webRoot, "index.html")
	files := http.FileServer(http.Dir(s.webRoot))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		path := filepath.Join(s.webRoot, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			files.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, distIndex)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeAPIError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	if os.IsNotExist(err) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	http.Error(w, err.Error(), http.StatusBadRequest)
}

func setCORSHeaders(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	switch origin {
	case "http://localhost:5173", "http://127.0.0.1:5173":
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	}
}

type manageLogResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *manageLogResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func logManageRequest(r *http.Request) {
	if r.URL.Path == "/api/healthz" {
		// Skip logging healthz requests to reduce noise
		return
	}
	log.Printf("%s request %s %s", masterLogPrefix, r.Method, fullManagePath(r))
}

func logManageResponse(r *http.Request, statusCode int, statusText string) {
	// Skip logging healthz responses to reduce noise
	if r.URL.Path == "/api/healthz" {
		return
	}
	color := masterLogOK
	if statusCode >= http.StatusBadRequest {
		color = masterLogFail
	}
	log.Printf("%s response %s %s %s[%d] %s%s", masterLogPrefix, r.Method, fullManagePath(r), color, statusCode, statusText, masterLogReset)
}

func fullManagePath(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	if r.URL.RawQuery == "" {
		return r.URL.Path
	}
	return r.URL.Path + "?" + r.URL.RawQuery
}

func tlsPayloadFromBuilder(route *ingressbuilder.TLSRouteBuilder) manageTLSPayload {
	if route == nil {
		return manageTLSPayload{}
	}
	return manageTLSPayload{
		Name:            route.HostName,
		SNI:             route.SNI,
		CertPEM:         route.CertPEM,
		KeyPEM:          route.KeyPEM,
		Kind:            route.Kind.String(),
		BackendHostname: route.BackendHostname,
		BackendPort:     route.BackendPort,
	}
}

func tlsRouteFromPayload(hostname string, input manageTLSPayload) (*ingressbuilder.TLSRouteBuilder, error) {
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		hostname = strings.TrimSpace(input.Name)
	}
	if hostname == "" {
		return nil, fmt.Errorf("hostname is required")
	}
	sni := strings.TrimSpace(input.SNI)
	if sni == "" {
		sni = hostname
	}
	route := ingressbuilder.NewTLSRoute().
		WithHostName(hostname).
		WithSNI(sni).
		WithCertPEM(input.CertPEM).
		WithKeyPEM(input.KeyPEM)

	kind, err := parseTLSKind(input.Kind)
	if err != nil {
		return nil, err
	}
	route.WithKind(kind)
	if input.BackendHostname != "" || input.BackendPort != 0 {
		route.WithBackend(input.BackendHostname, input.BackendPort)
	}
	return route, nil
}

func httpPayloadFromBuilder(route *ingressbuilder.HTTPRouteBuilder) manageHTTPPayload {
	out := manageHTTPPayload{}
	if route == nil {
		return out
	}
	out.Name = route.HostName
	out.Policies = make([]manageHTTPPolicy, 0, len(route.Policies))
	for _, policy := range route.Policies {
		if policy == nil {
			continue
		}
		next := manageHTTPPolicy{
			Backend:        policy.Backend,
			FixContent:     policy.FixContent,
			AllowRawAccess: policy.AllowRawAccess,
		}
		switch {
		case policy.Pathname != "":
			next.Pathname = policy.Pathname
			next.PathnameKind = policy.PathnameKind.String()
		case len(policy.QueryItems) > 0:
			next.QueryItems = make([]manageHTTPKeyValue, 0, len(policy.QueryItems))
			for _, item := range policy.QueryItems {
				next.QueryItems = append(next.QueryItems, manageHTTPKeyValue{Key: item.Key, Value: item.Value})
			}
		case len(policy.HeaderItems) > 0:
			next.HeaderItems = make([]manageHTTPKeyValue, 0, len(policy.HeaderItems))
			for _, item := range policy.HeaderItems {
				next.HeaderItems = append(next.HeaderItems, manageHTTPKeyValue{Key: item.Key, Value: item.Value})
			}
		}
		out.Policies = append(out.Policies, next)
	}
	return out
}

func httpRouteFromPayload(hostname string, input manageHTTPPayload) (*ingressbuilder.HTTPRouteBuilder, error) {
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		hostname = strings.TrimSpace(input.Name)
	}
	if hostname == "" {
		return nil, fmt.Errorf("hostname is required")
	}
	route := ingressbuilder.NewHTTPRoute().WithHostName(hostname)
	for _, policy := range input.Policies {
		next := ingressbuilder.NewHTTPPolicy().
			WithBackend(strings.TrimSpace(policy.Backend)).
			WithFixContent(policy.FixContent).
			WithAllowRawAccess(policy.AllowRawAccess)
		switch {
		case strings.TrimSpace(policy.Pathname) != "":
			switch strings.TrimSpace(policy.PathnameKind) {
			case "", "exact":
				next.WithExactPath(strings.TrimSpace(policy.Pathname))
			case "prefix":
				next.WithPrefixPath(strings.TrimSpace(policy.Pathname))
			case "regex":
				next.WithRegexPath(strings.TrimSpace(policy.Pathname))
			default:
				return nil, fmt.Errorf("unsupported pathname_kind: %q", policy.PathnameKind)
			}
		case len(policy.QueryItems) > 0:
			for _, item := range policy.QueryItems {
				next.WithQuery(item.Key, item.Value)
			}
		case len(policy.HeaderItems) > 0:
			for _, item := range policy.HeaderItems {
				next.WithHeader(item.Key, item.Value)
			}
		default:
			return nil, fmt.Errorf("one of pathname, query_items, header_items is required")
		}
		route.AddPolicy(next)
	}
	return route, nil
}

func parseTLSKind(value string) (ingress.TlsPolicy_Kind, error) {
	switch strings.TrimSpace(value) {
	case "", "https":
		return ingress.TlsPolicy_Kind_https, nil
	case "tlsPassthrough":
		return ingress.TlsPolicy_Kind_tlsPassthrough, nil
	case "tlsTerminate":
		return ingress.TlsPolicy_Kind_tlsTerminate, nil
	default:
		return 0, fmt.Errorf("unsupported tls kind: %q", value)
	}
}

func dnsPayloadToBin(input manageDNSPayload) ([]byte, error) {
	records := make([]dns.RecordBuilder, 0, len(input.Records))
	for i, record := range input.Records {
		builder, err := dnsRecordBuilderFromPayload(record)
		if err != nil {
			return nil, fmt.Errorf("build dns record %d: %w", i, err)
		}
		records = append(records, builder)
	}

	var buf bytes.Buffer
	if err := writeDNSRecords(&buf, records...); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func dnsPayloadFromBin(bin []byte) (manageDNSPayload, error) {
	zone, err := dns.Read(bytes.NewReader(bin))
	if err != nil {
		return manageDNSPayload{}, err
	}
	records, err := zone.Records()
	if err != nil {
		return manageDNSPayload{}, err
	}

	out := manageDNSPayload{
		Records: make([]manageDNSRecord, 0, records.Len()),
	}
	for i := 0; i < records.Len(); i++ {
		record, err := dnsRecordPayloadFromZone(records.At(i))
		if err != nil {
			return manageDNSPayload{}, fmt.Errorf("decode dns record %d: %w", i, err)
		}
		out.Records = append(out.Records, record)
	}
	return out, nil
}

func dnsRecordBuilderFromPayload(record manageDNSRecord) (dns.RecordBuilder, error) {
	switch strings.TrimSpace(record.Type) {
	case "a":
		builder := dnsbuilder.NewA().
			WithName(record.Name).
			WithAddress(record.Address)
		if record.TTL != 0 {
			builder.WithTTL(record.TTL)
		}
		return builder, nil
	case "aaaa":
		builder := dnsbuilder.NewAAAA().
			WithName(record.Name).
			WithAddress(record.Address)
		if record.TTL != 0 {
			builder.WithTTL(record.TTL)
		}
		return builder, nil
	case "cname":
		builder := dnsbuilder.NewCNAME().
			WithName(record.Name).
			WithHost(record.Host)
		if record.TTL != 0 {
			builder.WithTTL(record.TTL)
		}
		return builder, nil
	case "mx":
		builder := dnsbuilder.NewMX().
			WithName(record.Name).
			WithPreference(record.Preference).
			WithExchange(record.Exchange)
		if record.TTL != 0 {
			builder.WithTTL(record.TTL)
		}
		return builder, nil
	case "ns":
		builder := dnsbuilder.NewNS().
			WithName(record.Name).
			WithHost(record.Host)
		if record.TTL != 0 {
			builder.WithTTL(record.TTL)
		}
		return builder, nil
	case "ptr":
		builder := dnsbuilder.NewPTR().
			WithName(record.Name).
			WithHost(record.Host)
		if record.TTL != 0 {
			builder.WithTTL(record.TTL)
		}
		return builder, nil
	case "soa":
		builder := dnsbuilder.NewSOA().
			WithName(record.Name).
			WithMName(record.MName).
			WithRName(record.RName)
		if record.TTL != 0 {
			builder.WithTTL(record.TTL)
		}
		if record.Serial != 0 {
			builder.WithSerial(record.Serial)
		}
		if record.Refresh != 0 {
			builder.WithRefresh(record.Refresh)
		}
		if record.Retry != 0 {
			builder.WithRetry(record.Retry)
		}
		if record.Expire != 0 {
			builder.WithExpire(record.Expire)
		}
		if record.Minimum != 0 {
			builder.WithMinimum(record.Minimum)
		}
		return builder, nil
	case "txt":
		builder := dnsbuilder.NewTXT().
			WithName(record.Name).
			WithValues(record.Values...)
		if record.TTL != 0 {
			builder.WithTTL(record.TTL)
		}
		return builder, nil
	default:
		return nil, fmt.Errorf("unsupported dns record type: %q", record.Type)
	}
}

func dnsRecordPayloadFromZone(record dns.DnsRecord) (manageDNSRecord, error) {
	name, err := record.Name()
	if err != nil {
		return manageDNSRecord{}, err
	}

	out := manageDNSRecord{
		Type: record.Type().String(),
		Name: name,
		TTL:  record.Ttl(),
	}

	switch record.Which() {
	case dns.DnsRecord_Which_a:
		value, err := record.A()
		if err != nil {
			return manageDNSRecord{}, err
		}
		out.Address = net.IPv4(
			byte(value.Address()>>24),
			byte(value.Address()>>16),
			byte(value.Address()>>8),
			byte(value.Address()),
		).String()
	case dns.DnsRecord_Which_aaaa:
		value, err := record.Aaaa()
		if err != nil {
			return manageDNSRecord{}, err
		}
		ip := make(net.IP, net.IPv6len)
		binary.BigEndian.PutUint64(ip[:8], value.AddressHigh())
		binary.BigEndian.PutUint64(ip[8:], value.AddressLow())
		out.Address = ip.String()
	case dns.DnsRecord_Which_cname:
		value, err := record.Cname()
		if err != nil {
			return manageDNSRecord{}, err
		}
		out.Host, err = value.Host()
		if err != nil {
			return manageDNSRecord{}, err
		}
	case dns.DnsRecord_Which_mx:
		value, err := record.Mx()
		if err != nil {
			return manageDNSRecord{}, err
		}
		out.Preference = value.Preference()
		out.Exchange, err = value.Exchange()
		if err != nil {
			return manageDNSRecord{}, err
		}
	case dns.DnsRecord_Which_ns:
		value, err := record.Ns()
		if err != nil {
			return manageDNSRecord{}, err
		}
		out.Host, err = value.Host()
		if err != nil {
			return manageDNSRecord{}, err
		}
	case dns.DnsRecord_Which_ptr:
		value, err := record.Ptr()
		if err != nil {
			return manageDNSRecord{}, err
		}
		out.Host, err = value.Host()
		if err != nil {
			return manageDNSRecord{}, err
		}
	case dns.DnsRecord_Which_soa:
		value, err := record.Soa()
		if err != nil {
			return manageDNSRecord{}, err
		}
		out.MName, err = value.Mname()
		if err != nil {
			return manageDNSRecord{}, err
		}
		out.RName, err = value.Rname()
		if err != nil {
			return manageDNSRecord{}, err
		}
		out.Serial = value.Serial()
		out.Refresh = value.Refresh()
		out.Retry = value.Retry()
		out.Expire = value.Expire()
		out.Minimum = value.Minimum()
	case dns.DnsRecord_Which_txt:
		value, err := record.Txt()
		if err != nil {
			return manageDNSRecord{}, err
		}
		list, err := value.Values()
		if err != nil {
			return manageDNSRecord{}, err
		}
		out.Values = make([]string, 0, list.Len())
		for i := 0; i < list.Len(); i++ {
			item, err := list.At(i)
			if err != nil {
				return manageDNSRecord{}, err
			}
			out.Values = append(out.Values, item)
		}
	default:
		return manageDNSRecord{}, fmt.Errorf("unsupported dns union: %v", record.Which())
	}

	return out, nil
}
