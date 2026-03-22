package acme

import (
	"context"
	"crypto"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
)

const (
	LetsEncryptProductionURL = "https://acme-v02.api.letsencrypt.org/directory"
	ZeroSSLProductionURL     = "https://acme.zerossl.com/v2/DV90"
)

type Provider string

const (
	ProviderLetsEncrypt Provider = "letsencrypt"
	ProviderZeroSSL     Provider = "zerossl"
)

type legoUser struct {
	email        string
	key          crypto.PrivateKey
	registration *registration.Resource
}

func (u *legoUser) GetEmail() string {
	return u.email
}

func (u *legoUser) GetRegistration() *registration.Resource {
	return u.registration
}

func (u *legoUser) GetPrivateKey() crypto.PrivateKey {
	return u.key
}

type http01Provider struct {
	solver   *HTTP01Solver
	hostname string
}

func (p *http01Provider) Present(domain, token, keyAuth string) error {
	return p.solver.Present(context.Background(), p.hostname, token, keyAuth)
}

func (p *http01Provider) CleanUp(domain, token, keyAuth string) error {
	return p.solver.Cleanup(context.Background(), p.hostname, token)
}

type legoIssuer struct {
	client   *lego.Client
	provider challenge.Provider
}

func newLegoIssuer(email string, accountKey crypto.PrivateKey, provider string, httpClient *http.Client, eabKID, eabHMAC string, challengeProvider challenge.Provider) (*legoIssuer, error) {
	logACMEStart("issuer init provider=%s", provider)
	if strings.TrimSpace(email) == "" {
		err := fmt.Errorf("email is required")
		logACMEError(err, "issuer init provider=%s", provider)
		return nil, err
	}
	if accountKey == nil {
		err := fmt.Errorf("account key is required")
		logACMEError(err, "issuer init provider=%s", provider)
		return nil, err
	}
	if challengeProvider == nil {
		err := fmt.Errorf("challenge provider is required")
		logACMEError(err, "issuer init provider=%s", provider)
		return nil, err
	}

	user := &legoUser{
		email: strings.TrimSpace(email),
		key:   accountKey,
	}
	config := lego.NewConfig(user)
	config.Certificate.KeyType = certcrypto.EC256
	if httpClient != nil {
		config.HTTPClient = httpClient
	} else {
		config.HTTPClient = http.DefaultClient
	}

	switch Provider(strings.TrimSpace(provider)) {
	case "", ProviderZeroSSL:
		config.CADirURL = ZeroSSLProductionURL
		if strings.TrimSpace(eabKID) == "" || strings.TrimSpace(eabHMAC) == "" {
			err := fmt.Errorf("zerossl requires external account binding")
			logACMEError(err, "issuer init provider=%s", provider)
			return nil, err
		}
	case ProviderLetsEncrypt:
		config.CADirURL = LetsEncryptProductionURL
	default:
		err := fmt.Errorf("unsupported acme provider: %q", provider)
		logACMEError(err, "issuer init provider=%s", provider)
		return nil, err
	}

	client, err := lego.NewClient(config)
	if err != nil {
		logACMEError(err, "issuer init provider=%s", provider)
		return nil, err
	}

	switch Provider(strings.TrimSpace(provider)) {
	case "", ProviderZeroSSL:
		reg, err := client.Registration.RegisterWithExternalAccountBinding(registration.RegisterEABOptions{
			TermsOfServiceAgreed: true,
			Kid:                  strings.TrimSpace(eabKID),
			HmacEncoded:          strings.TrimSpace(eabHMAC),
		})
		if err != nil {
			reg, err = client.Registration.ResolveAccountByKey()
			if err != nil {
				logACMEError(err, "issuer register provider=%s email=%s", provider, user.email)
				return nil, err
			}
		}
		user.registration = reg
	case ProviderLetsEncrypt:
		reg, err := client.Registration.Register(registration.RegisterOptions{
			TermsOfServiceAgreed: true,
		})
		if err != nil {
			reg, err = client.Registration.ResolveAccountByKey()
			if err != nil {
				logACMEError(err, "issuer register provider=%s email=%s", provider, user.email)
				return nil, err
			}
		}
		user.registration = reg
	}

	logACMEDone("issuer init provider=%s directory=%s", provider, config.CADirURL)
	return &legoIssuer{
		client:   client,
		provider: challengeProvider,
	}, nil
}

func (i *legoIssuer) Obtain(_ context.Context, domains []string) (*certificate.Resource, error) {
	if len(domains) == 0 {
		return nil, fmt.Errorf("domains are required")
	}
	if err := i.client.Challenge.SetHTTP01Provider(i.provider); err != nil {
		return nil, err
	}
	return i.client.Certificate.Obtain(certificate.ObtainRequest{
		Domains: domains,
		Bundle:  true,
	})
}
