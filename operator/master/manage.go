package master

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
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
			return s.master.ReadDNSJSON(ctx, hostname)
		},
		func(ctx context.Context, hostname string, payload []byte) (any, error) {
			return s.master.PublishDNSJSON(ctx, hostname, payload)
		},
	)
}

func (s *ManageServer) handleTLS(w http.ResponseWriter, r *http.Request) {
	s.handleResource(
		w,
		r,
		func(ctx context.Context, hostname string) (any, error) {
			return s.master.ReadTLSJSON(ctx, hostname)
		},
		func(ctx context.Context, hostname string, payload []byte) (any, error) {
			return s.master.PublishTLSJSON(ctx, hostname, payload)
		},
	)
}

func (s *ManageServer) handleHTTP(w http.ResponseWriter, r *http.Request) {
	s.handleResource(
		w,
		r,
		func(ctx context.Context, hostname string) (any, error) {
			return s.master.ReadHTTPJSON(ctx, hostname)
		},
		func(ctx context.Context, hostname string, payload []byte) (any, error) {
			return s.master.PublishHTTPJSON(ctx, hostname, payload)
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
