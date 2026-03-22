package acme

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	ingress "aaa/ingress"
	ingressbuilder "aaa/ingress/builder"
)

type Config struct {
	DefaultProvider string
	DefaultEmail    string
	ZeroSSLEABKID   string
	ZeroSSLEABHMAC  string
}

type issueState struct {
	mu sync.Mutex
}

var issueStates sync.Map

const renewBefore = 30 * 24 * time.Hour

func issueLock(hostname string) *sync.Mutex {
	key := sanitizeKey(hostname)
	stateAny, _ := issueStates.LoadOrStore(key, &issueState{})
	return &stateAny.(*issueState).mu
}

func HandleHTTPPublish(ctx context.Context, c Controller, cfg Config, hostname string, route *ingressbuilder.HTTPRouteBuilder) error {
	logACMEStart("auto-request [%s]", hostname)
	if c == nil || hostname == "" || route == nil || len(route.Policies) == 0 || isACMEChallengeRoute(route) {
		logACMEDone("auto-request skipped [%s]", hostname)
		return nil
	}
	if strings.TrimSpace(cfg.DefaultEmail) == "" {
		logACMEDone("auto-request skipped [%s] reason=missing-email", hostname)
		return nil
	}

	req := certificateRequest{
		Hostname:  hostname,
		Status:    "pending",
		Provider:  providerName(cfg),
		Challenge: string(challengeTypeHTTP01),
		CreatedAt: time.Now().Unix(),
	}
	requestPath := filepath.Join(c.Root(), "requests", sanitizeKey(hostname)+".json")
	if err := saveJSONFile(requestPath, req); err != nil {
		logACMEError(err, "auto-request [%s]", hostname)
		return err
	}

	lock := issueLock(hostname)
	lock.Lock()
	defer lock.Unlock()

	issueCtx := ctx
	if issueCtx == nil {
		issueCtx = context.Background()
	}
	var cancel context.CancelFunc
	issueCtx, cancel = context.WithTimeout(issueCtx, 5*time.Minute)
	defer cancel()

	currentTLS, err := c.ReadTLSRoute(issueCtx, hostname)
	if err == nil && !certificateNeedsRenewal(currentTLS.CertPEM) {
		req.Status = "reused"
		_ = saveJSONFile(requestPath, req)
		logACMEDone("auto-request skipped [%s] reason=tls-bin-valid", hostname)
		return nil
	}
	if err != nil && !isNotExist(err) {
		logACMEError(err, "auto-request [%s]", hostname)
		return err
	}

	if err := issueAndPublishTLS(issueCtx, c, cfg, hostname); err != nil {
		req.Status = "error"
		_ = saveJSONFile(requestPath, req)
		logACMEError(err, "auto-request [%s]", hostname)
		return err
	}

	req.Status = "issued"
	if err := saveJSONFile(requestPath, req); err != nil {
		logACMEError(err, "auto-request [%s]", hostname)
		return err
	}
	logACMEDone("auto-request [%s] challenge=http-01 provider=%s", hostname, req.Provider)
	return nil
}

func issueAndPublishTLS(ctx context.Context, c Controller, cfg Config, hostname string) error {
	provider := providerName(cfg)
	accountKey, err := loadOrCreateECDSAKey(filepath.Join(
		c.Root(),
		"accounts",
		sanitizeKey(provider)+".pem",
	))
	if err != nil {
		return fmt.Errorf("load acme account key: %w", err)
	}
	issuer, err := newLegoIssuer(
		strings.TrimSpace(cfg.DefaultEmail),
		accountKey,
		provider,
		nil,
		strings.TrimSpace(cfg.ZeroSSLEABKID),
		strings.TrimSpace(cfg.ZeroSSLEABHMAC),
		&http01Provider{
			solver:   New(c).HTTP01(),
			hostname: hostname,
		},
	)
	if err != nil {
		return err
	}
	resource, err := issuer.Obtain(ctx, []string{hostname})
	if err != nil {
		return err
	}
	certPEM := string(resource.Certificate)
	keyPEM := string(resource.PrivateKey)

	next := ingressbuilder.NewTLSRoute().
		WithHostName(hostname).
		WithSNI(hostname).
		WithKind(ingress.TlsPolicy_Kind_https).
		WithCertPEM(certPEM).
		WithKeyPEM(keyPEM)
	current, err := c.ReadTLSRoute(ctx, hostname)
	if err == nil {
		if current.Kind != 0 {
			next.Kind = current.Kind
		}
		if strings.TrimSpace(current.BackendHostname) != "" {
			next.BackendHostname = current.BackendHostname
		}
		if current.BackendPort != 0 {
			next.BackendPort = current.BackendPort
		}
	} else if !isNotExist(err) {
		return fmt.Errorf("read existing tls: %w", err)
	}

	if next.Kind == 0 {
		next.Kind = ingress.TlsPolicy_Kind_https
	}
	return c.PublishTLSRoute(ctx, hostname, next)
}

func providerName(cfg Config) string {
	provider := strings.TrimSpace(cfg.DefaultProvider)
	if provider == "" {
		return string(ProviderZeroSSL)
	}
	return provider
}

func certificateNeedsRenewal(certPEM string) bool {
	notAfter, err := certificateNotAfter(certPEM)
	if err != nil {
		return true
	}
	return time.Until(notAfter) <= renewBefore
}

func certificateNotAfter(certPEM string) (time.Time, error) {
	rest := []byte(certPEM)
	for len(rest) > 0 {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return time.Time{}, err
		}
		return cert.NotAfter, nil
	}
	return time.Time{}, fmt.Errorf("certificate pem not found")
}

func loadOrCreateECDSAKey(path string) (*ecdsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("decode pem key: empty block")
		}
		key, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse ec private key: %w", err)
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ec private key: %w", err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal ec private key: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		return nil, err
	}
	return key, nil
}
