package acme

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	acmeLogPrefix = "\033[38;5;45m[ACME]\033[0m"
	acmeStateDir  = "operator/master/acme"
)

type Manager struct {
	controller Controller
}

type challengeType string

const (
	challengeTypeHTTP01 challengeType = "http-01"
)

type challengeState struct {
	Type       challengeType `json:"type"`
	Hostname   string        `json:"hostname"`
	Token      string        `json:"token"`
	FixContent string        `json:"fix_content"`
	CreatedAt  int64         `json:"created_at"`
}

type certificateRequest struct {
	Hostname  string `json:"hostname"`
	Status    string `json:"status"`
	Provider  string `json:"provider"`
	Challenge string `json:"challenge"`
	CreatedAt int64  `json:"created_at"`
}

type HTTPPolicy struct {
	Backend        string
	PathnameKind   string
	Pathname       string
	QueryItems     []HTTPKV
	HeaderItems    []HTTPKV
	FixContent     string
	AllowRawAccess bool
}

type HTTPKV struct {
	Key   string
	Value string
}

type HTTPChange struct {
	Name     string
	Policies []HTTPPolicy
}

type TLSChange struct {
	CertPEM string
	KeyPEM  string
}

type Controller interface {
	Root() string
	ReadHTTP(context.Context, string) (HTTPChange, error)
	PublishHTTPChange(context.Context, string, HTTPChange) error
	ReadTLS(context.Context, string) (TLSChange, error)
}

func New(controller Controller) *Manager {
	return &Manager{controller: controller}
}

func (m *Manager) HTTP01() *HTTP01Solver {
	return &HTTP01Solver{controller: m.controller}
}

func ensureController(c Controller) error {
	if c == nil {
		return fmt.Errorf("acme controller is required")
	}
	return nil
}

func run(ctx context.Context, fn func(context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return fn(ctx)
}

func isNotExist(err error) bool {
	return err != nil && os.IsNotExist(err)
}

func HandleHTTPPublish(ctx context.Context, c Controller, hostname string, change HTTPChange) error {
	if c == nil || hostname == "" || len(change.Policies) == 0 || isACMEChallengeChange(change) {
		return nil
	}
	tlsChange, err := c.ReadTLS(ctx, hostname)
	if err == nil && strings.TrimSpace(tlsChange.CertPEM) != "" && strings.TrimSpace(tlsChange.KeyPEM) != "" {
		return nil
	}
	if err != nil && !isNotExist(err) {
		return err
	}

	req := certificateRequest{
		Hostname:  hostname,
		Status:    "pending",
		Provider:  string(ProviderLetsEncrypt),
		Challenge: string(challengeTypeHTTP01),
		CreatedAt: time.Now().Unix(),
	}
	if err := saveJSONFile(filepath.Join(c.Root(), acmeStateDir, "requests", sanitizeKey(hostname)+".json"), req); err != nil {
		return err
	}
	log.Printf("%s auto request hostname=%s challenge=http-01 provider=%s", acmeLogPrefix, hostname, req.Provider)
	return nil
}

func isACMEChallengeChange(change HTTPChange) bool {
	for _, policy := range change.Policies {
		if strings.HasPrefix(policy.Pathname, "/.well-known/acme-challenge/") && policy.FixContent != "" {
			return true
		}
	}
	return false
}

func saveChallengeState(root string, state challengeState, key string) error {
	return saveJSONFile(filepath.Join(root, acmeStateDir, "http01", sanitizeKey(key)+".json"), state)
}

func loadChallengeState(root, key string) (challengeState, error) {
	var state challengeState
	data, err := os.ReadFile(filepath.Join(root, acmeStateDir, "http01", sanitizeKey(key)+".json"))
	if err != nil {
		return challengeState{}, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return challengeState{}, err
	}
	return state, nil
}

func deleteChallengeState(root, key string) error {
	err := os.Remove(filepath.Join(root, acmeStateDir, "http01", sanitizeKey(key)+".json"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func saveJSONFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func sanitizeKey(key string) string {
	key = strings.TrimSpace(strings.ToLower(key))
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "?", "_", "&", "_", "=", "_", "|", "_")
	key = replacer.Replace(key)
	if key == "" {
		return "empty"
	}
	return key
}
