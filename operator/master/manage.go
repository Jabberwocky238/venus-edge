package master

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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
	WAL         WALStatus            `json:"wal"`
	WALFiles    []manageFileInfo     `json:"wal_files"`
}

type manageFileInfo struct {
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"mod_time"`
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
	return s.mux
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
	s.mux.HandleFunc("/api/master/overview", s.handleOverview)
	s.mux.HandleFunc("/api/master/wal", s.handleWAL)
	s.mux.Handle("/", s.staticHandler())
}

func (s *ManageServer) handleOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status := s.master.WALStatus()
	writeJSON(w, manageOverview{
		NowUnix:     time.Now().Unix(),
		Subscribers: s.hub.Snapshot(),
		WAL:         status,
		WALFiles:    statFiles(s.master.WALFiles()),
	})
}

func (s *ManageServer) handleWAL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{
		"wal":   s.master.WALStatus(),
		"files": statFiles(s.master.WALFiles()),
	})
}

func (s *ManageServer) staticHandler() http.Handler {
	distIndex := filepath.Join(s.webRoot, "index.html")
	files := http.FileServer(http.Dir(s.webRoot))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func statFiles(paths []string) []manageFileInfo {
	files := make([]manageFileInfo, 0, len(paths))
	for _, path := range paths {
		info := manageFileInfo{Path: path}
		if stat, err := os.Stat(path); err == nil {
			info.Exists = true
			info.Size = stat.Size()
			info.ModTime = stat.ModTime().Unix()
		}
		files = append(files, info)
	}
	return files
}
