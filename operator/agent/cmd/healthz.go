package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	dns "aaa/DNS"
	ingress "aaa/ingress"
)

type healthServer struct {
	addr     string
	ingress  *ingress.Engine
	dns      *dns.Engine
	tlsAddr  string
	httpAddr string
	dnsAddr  string
}

type healthResponse struct {
	OK         bool   `json:"ok"`
	NowUnix    int64  `json:"now_unix"`
	TLSReady   bool   `json:"tls_ready"`
	HTTPReady  bool   `json:"http_ready"`
	DNSReady   bool   `json:"dns_ready"`
	TLSListen  string `json:"tls_listen,omitempty"`
	HTTPListen string `json:"http_listen,omitempty"`
	DNSListen  string `json:"dns_listen,omitempty"`
}

func newHealthServer(addr string, ingressEngine *ingress.Engine, dnsEngine *dns.Engine, tlsAddr, httpAddr, dnsAddr string) *healthServer {
	return &healthServer{
		addr:     addr,
		ingress:  ingressEngine,
		dns:      dnsEngine,
		tlsAddr:  tlsAddr,
		httpAddr: httpAddr,
		dnsAddr:  dnsAddr,
	}
}

func (s *healthServer) Listen(ctx context.Context) error {
	if s == nil || s.addr == "" {
		return nil
	}
	server := &http.Server{
		Addr:              s.addr,
		Handler:           s.handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	err := server.ListenAndServe()
	if err == nil || err == http.ErrServerClosed || ctx.Err() != nil {
		return nil
	}
	return err
}

func (s *healthServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/api/healthz", s.handleHealthz)
	return mux
}

func (s *healthServer) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp := healthResponse{
		NowUnix:    time.Now().Unix(),
		TLSReady:   s.tlsAddr == "" || (s.ingress != nil && s.ingress.Addr() != nil),
		HTTPReady:  s.httpAddr == "" || (s.ingress != nil && s.ingress.HTTPAddr() != nil),
		DNSReady:   s.dnsAddr == "" || (s.dns != nil && s.dns.Addr() != ""),
		TLSListen:  s.tlsAddr,
		HTTPListen: s.httpAddr,
		DNSListen:  s.dnsAddr,
	}
	resp.OK = resp.TLSReady && resp.HTTPReady && resp.DNSReady
	if !resp.OK {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
