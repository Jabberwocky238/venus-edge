package acme

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"net/http"
	"strings"

	xacme "golang.org/x/crypto/acme"
)

const (
	LetsEncryptProductionURL = xacme.LetsEncryptURL
	LetsEncryptStagingURL    = "https://acme-staging-v02.api.letsencrypt.org/directory"
	ZeroSSLProductionURL     = "https://acme.zerossl.com/v2/DV90"
	ZeroSSLStagingURL        = "https://acme.zerossl.com/v2/DV90/test"
)

type Provider string

const (
	ProviderLetsEncrypt Provider = "letsencrypt"
	ProviderZeroSSL     Provider = "zerossl"
)

type ExternalAccount struct {
	KID string
	Key []byte
}

type IssuerConfig struct {
	Provider      Provider
	DirectoryURL  string
	Email         string
	Staging       bool
	AccountKey    crypto.Signer
	UserAgent     string
	HTTPClient    *http.Client
	Prompt        func(string) bool
	ExternalAccount *ExternalAccount
}

type Issuer struct {
	config IssuerConfig
	client *xacme.Client
}

func NewIssuer(cfg IssuerConfig) (*Issuer, error) {
	logACMEStart("issuer init provider=%s staging=%t", cfg.Provider, cfg.Staging)
	if strings.TrimSpace(cfg.Email) == "" {
		err := fmt.Errorf("email is required")
		logACMEError(err, "issuer init provider=%s staging=%t", cfg.Provider, cfg.Staging)
		return nil, err
	}

	dirURL, err := resolveDirectoryURL(cfg)
	if err != nil {
		logACMEError(err, "issuer init provider=%s staging=%t", cfg.Provider, cfg.Staging)
		return nil, err
	}
	cfg.DirectoryURL = dirURL

	if cfg.AccountKey == nil {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			logACMEError(err, "issuer init provider=%s staging=%t", cfg.Provider, cfg.Staging)
			return nil, fmt.Errorf("generate account key: %w", err)
		}
		cfg.AccountKey = key
	}

	client := &xacme.Client{
		Key:          cfg.AccountKey,
		DirectoryURL: cfg.DirectoryURL,
		HTTPClient:   cfg.HTTPClient,
		UserAgent:    cfg.UserAgent,
	}

	issuer := &Issuer{
		config: cfg,
		client: client,
	}
	logACMEDone("issuer init provider=%s directory=%s", cfg.Provider, cfg.DirectoryURL)
	return issuer, nil
}

func (i *Issuer) Client() *xacme.Client {
	if i == nil {
		return nil
	}
	return i.client
}

func (i *Issuer) Config() IssuerConfig {
	if i == nil {
		return IssuerConfig{}
	}
	return i.config
}

func (i *Issuer) Register(ctx context.Context) (*xacme.Account, error) {
	if i == nil || i.client == nil {
		err := fmt.Errorf("issuer is not initialized")
		logACMEError(err, "issuer register")
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	logACMEStart("issuer register provider=%s email=%s", i.config.Provider, strings.TrimSpace(i.config.Email))

	acct := &xacme.Account{
		Contact: []string{"mailto:" + strings.TrimSpace(i.config.Email)},
	}
	if i.config.ExternalAccount != nil {
		acct.ExternalAccountBinding = &xacme.ExternalAccountBinding{
			KID: i.config.ExternalAccount.KID,
			Key: i.config.ExternalAccount.Key,
		}
	}

	prompt := i.config.Prompt
	if prompt == nil {
		prompt = xacme.AcceptTOS
	}
	account, err := i.client.Register(ctx, acct, prompt)
	if err != nil {
		logACMEError(err, "issuer register provider=%s email=%s", i.config.Provider, strings.TrimSpace(i.config.Email))
		return nil, err
	}
	logACMEDone("issuer register provider=%s email=%s", i.config.Provider, strings.TrimSpace(i.config.Email))
	return account, nil
}

func resolveDirectoryURL(cfg IssuerConfig) (string, error) {
	if strings.TrimSpace(cfg.DirectoryURL) != "" {
		return cfg.DirectoryURL, nil
	}

	switch cfg.Provider {
	case ProviderLetsEncrypt, "":
		if cfg.Staging {
			return LetsEncryptStagingURL, nil
		}
		return LetsEncryptProductionURL, nil
	case ProviderZeroSSL:
		if cfg.ExternalAccount == nil || strings.TrimSpace(cfg.ExternalAccount.KID) == "" || len(cfg.ExternalAccount.Key) == 0 {
			return "", fmt.Errorf("zerossl requires external account binding")
		}
		if cfg.Staging {
			return ZeroSSLStagingURL, nil
		}
		return ZeroSSLProductionURL, nil
	default:
		return "", fmt.Errorf("unsupported acme provider: %q", cfg.Provider)
	}
}
