package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	masterpkg "aaa/operator/master"
	"aaa/operator/master/objectstore"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var (
		root              = flag.String("root", envOrDefault("VENUS_MASTER_ROOT", "."), "workspace root for master data")
		manageAddr        = flag.String("manage-addr", envOrDefault("VENUS_MANAGE_LISTEN", "127.0.0.1:9000"), "master manage api listen address")
		grpcAddr          = flag.String("grpc-addr", envOrDefault("VENUS_REPLICATION_LISTEN", "127.0.0.1:10992"), "master grpc listen address")
		webRoot           = flag.String("web-root", envOrDefault("VENUS_WEB_ROOT", "./operator/web/dist"), "optional web dist root")
		acmeProvider      = flag.String("acme-provider", envOrDefault("VENUS_ACME_DEFAULT_PROVIDER", "letsencrypt"), "default ACME provider")
		acmeEmail         = flag.String("acme-email", envOrDefault("VENUS_ACME_DEFAULT_EMAIL", ""), "default ACME account email")
		zeroSSLEABKID     = flag.String("zerossl-eab-kid", envOrDefault("VENUS_ACME_EAB_KID", ""), "ZeroSSL external account binding KID")
		zeroSSLEABHMACKey = flag.String("zerossl-eab-hmac-key", envOrDefault("VENUS_ACME_EAB_HMAC_KEY", ""), "ZeroSSL external account binding HMAC key")
	)
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	storeRoot := filepath.Join(*root, ".venus-edge", "master", "s3")
	store := objectstore.NewLocalFS(storeRoot)

	m, err := masterpkg.New(masterpkg.Options{
		Root:       *root,
		Store:      store,
		ManageAddr: *manageAddr,
		GRPCAddr:   *grpcAddr,
		WebRoot:    *webRoot,
		ACME: masterpkg.ACMEConfig{
			DefaultProvider: *acmeProvider,
			DefaultEmail:    *acmeEmail,
			ZeroSSLEABKID:   *zeroSSLEABKID,
			ZeroSSLEABHMAC:  *zeroSSLEABHMACKey,
		},
	})
	if err != nil {
		return err
	}

	return m.Start(ctx)
}

func envOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	switch value {
	case "1", "true", "TRUE", "yes", "YES", "on", "ON":
		return true
	case "0", "false", "FALSE", "no", "NO", "off", "OFF":
		return false
	default:
		return fallback
	}
}
