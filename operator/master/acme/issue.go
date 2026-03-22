package acme

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	xacme "golang.org/x/crypto/acme"
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

func HandleHTTPPublish(ctx context.Context, c Controller, cfg Config, hostname string, change HTTPChange) error {
	logACMEStart("auto-request [%s]", hostname)
	if c == nil || hostname == "" || len(change.Policies) == 0 || isACMEChallengeChange(change) {
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
		Provider:  defaultProvider(cfg),
		Challenge: string(challengeTypeHTTP01),
		CreatedAt: time.Now().Unix(),
	}
	requestPath := filepath.Join(c.Root(), acmeStateDir, "requests", sanitizeKey(hostname)+".json")
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

	currentTLS, err := c.ReadTLS(issueCtx, hostname)
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
	provider := strings.TrimSpace(cfg.DefaultProvider)
	if provider == "" {
		provider = string(ProviderZeroSSL)
	}
	accountKey, err := loadOrCreateECDSAKey(filepath.Join(
		c.Root(),
		acmeStateDir,
		"accounts",
		sanitizeKey(provider)+".pem",
	))
	if err != nil {
		return fmt.Errorf("load acme account key: %w", err)
	}
	var externalAccount *ExternalAccount
	if strings.TrimSpace(cfg.ZeroSSLEABKID) != "" && strings.TrimSpace(cfg.ZeroSSLEABHMAC) != "" {
		externalAccount = &ExternalAccount{
			KID: strings.TrimSpace(cfg.ZeroSSLEABKID),
			Key: []byte(strings.TrimSpace(cfg.ZeroSSLEABHMAC)),
		}
	}
	issuer, err := NewIssuer(IssuerConfig{
		Provider:        Provider(provider),
		Email:           strings.TrimSpace(cfg.DefaultEmail),
		AccountKey:      accountKey,
		UserAgent:       "venus-edge-master-acme",
		ExternalAccount: externalAccount,
	})
	if err != nil {
		return err
	}
	if _, err := issuer.Register(ctx); err != nil && !errors.Is(err, xacme.ErrAccountAlreadyExists) {
		return err
	}
	client := issuer.Client()
	if client == nil {
		return fmt.Errorf("acme client is not initialized")
	}

	order, err := client.AuthorizeOrder(ctx, xacme.DomainIDs(hostname))
	if err != nil {
		return fmt.Errorf("authorize order: %w", err)
	}
	solver := New(c).HTTP01()
	for _, authzURL := range order.AuthzURLs {
		authz, err := client.GetAuthorization(ctx, authzURL)
		if err != nil {
			return fmt.Errorf("get authorization: %w", err)
		}
		if authz.Status == xacme.StatusValid {
			continue
		}

		var challenge *xacme.Challenge
		for _, item := range authz.Challenges {
			if item != nil && item.Type == string(challengeTypeHTTP01) {
				challenge = item
				break
			}
		}
		if challenge == nil {
			return fmt.Errorf("http-01 challenge not offered for %s", authz.Identifier.Value)
		}

		response, err := client.HTTP01ChallengeResponse(challenge.Token)
		if err != nil {
			return fmt.Errorf("build http-01 response: %w", err)
		}
		if err := solver.Present(ctx, hostname, challenge.Token, response); err != nil {
			return fmt.Errorf("present http-01 challenge: %w", err)
		}

		waitErr := func() error {
			defer func() {
				_ = solver.Cleanup(context.Background(), hostname, challenge.Token)
			}()
			if _, err := client.Accept(ctx, challenge); err != nil {
				return fmt.Errorf("accept challenge: %w", err)
			}
			if _, err := client.WaitAuthorization(ctx, authz.URI); err != nil {
				return fmt.Errorf("wait authorization: %w", err)
			}
			return nil
		}()
		if waitErr != nil {
			return waitErr
		}
	}

	key, csrDER, err := createLeafCSR(hostname)
	if err != nil {
		return err
	}
	chain, _, err := client.CreateOrderCert(ctx, order.FinalizeURL, csrDER, true)
	if err != nil {
		return fmt.Errorf("create order cert: %w", err)
	}
	certPEM, err := encodeCertChainPEM(chain)
	if err != nil {
		return err
	}
	keyPEM, err := encodePrivateKeyPEM(key)
	if err != nil {
		return err
	}

	next := TLSChange{
		Name:    hostname,
		SNI:     hostname,
		Kind:    "https",
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
	}
	current, err := c.ReadTLS(ctx, hostname)
	if err == nil {
		if strings.TrimSpace(current.Kind) != "" {
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

	if strings.TrimSpace(next.Kind) == "" {
		next.Kind = "https"
	}
	return c.PublishTLSChange(ctx, hostname, next)
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

func createLeafCSR(hostname string) (*ecdsa.PrivateKey, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate leaf key: %w", err)
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: hostname},
		DNSNames: []string{hostname},
	}, key)
	if err != nil {
		return nil, nil, fmt.Errorf("create certificate request: %w", err)
	}
	return key, csrDER, nil
}

func encodeCertChainPEM(chain [][]byte) (string, error) {
	var out []byte
	for _, der := range chain {
		out = append(out, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	if len(out) == 0 {
		return "", fmt.Errorf("empty certificate chain")
	}
	return string(out), nil
}

func encodePrivateKeyPEM(key *ecdsa.PrivateKey) (string, error) {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", fmt.Errorf("marshal leaf private key: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})), nil
}
