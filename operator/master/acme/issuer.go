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
	if strings.TrimSpace(cfg.Email) == "" {
		return nil, fmt.Errorf("email is required")
	}

	dirURL, err := resolveDirectoryURL(cfg)
	if err != nil {
		return nil, err
	}
	cfg.DirectoryURL = dirURL

	if cfg.AccountKey == nil {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
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

	return &Issuer{
		config: cfg,
		client: client,
	}, nil
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
		return nil, fmt.Errorf("issuer is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

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
	return i.client.Register(ctx, acct, prompt)
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
