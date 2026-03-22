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
	_ "aaa/operator/master/acme"
	"aaa/operator/master/objectstore"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var (
		root       = flag.String("root", ".", "workspace root for master data")
		manageAddr = flag.String("manage-addr", "127.0.0.1:9000", "master manage api listen address")
		grpcAddr   = flag.String("grpc-addr", "127.0.0.1:10992", "master grpc listen address")
		webRoot    = flag.String("web-root", "", "optional web dist root")
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
	})
	if err != nil {
		return err
	}

	return m.Start(ctx)
}
