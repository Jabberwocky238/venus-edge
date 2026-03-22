package acme

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	acmeLogPrefix = "\033[38;5;226m[ACME]\033[0m"
	acmeLogOK     = "\033[32m"
	acmeLogFail   = "\033[31m"
	acmeLogReset  = "\033[0m"
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

type Controller interface {
	Root() string
	ReadHTTP(context.Context, string) (HTTPChange, error)
	PublishHTTPChange(context.Context, string, HTTPChange) error
	ReadTLS(context.Context, string) (TLSChange, error)
	PublishTLSChange(context.Context, string, TLSChange) error
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

func logACMEStart(format string, args ...any) {
	log.Printf("%s start "+format, append([]any{acmeLogPrefix}, args...)...)
}

func logACMEDone(format string, args ...any) {
	log.Printf("%s %sdone%s "+format, append([]any{acmeLogPrefix, acmeLogOK, acmeLogReset}, args...)...)
}

func logACMEError(err error, format string, args ...any) {
	if err == nil {
		return
	}
	args = append(args, err)
	log.Printf("%s %serror%s "+format+" err=%v", append([]any{acmeLogPrefix, acmeLogFail, acmeLogReset}, args...)...)
}
