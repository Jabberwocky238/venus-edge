package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	dns "aaa/DNS"
	ingress "aaa/ingress"
	"aaa/operator/agent"
	"aaa/operator/replication"

	"golang.org/x/sync/errgroup"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var (
		root      = flag.String("root", ".", "workspace root for agent data")
		tlsAddr   = flag.String("tls-addr", "127.0.0.1:8443", "TLS engine listen address")
		httpAddr  = flag.String("http-addr", "127.0.0.1:8080", "HTTP engine listen address")
		dnsAddr   = flag.String("dns-addr", "127.0.0.1:8053", "DNS engine listen address")
		masterURL = flag.String("master-url", "127.0.0.1:10992", "operator master replication endpoint")
		podIP     = flag.String("pod-ip", "127.0.0.1", "subscriber pod IP for master replication")
		agentID   = flag.String("agent-id", "agent", "subscriber agent ID for master replication")
		mmdbPath  = flag.String("mmdb-path", "./data/GeoLite2-City.mmdb", "optional GeoIP MMDB path for DNS engine")
	)
	flag.Parse()

	if *tlsAddr == "" && *httpAddr == "" && *dnsAddr == "" {
		return fmt.Errorf("at least one of -tls-addr, -http-addr, or -dns-addr is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ag, err := agent.New(*root)
	if err != nil {
		return fmt.Errorf("init agent: %w", err)
	}

	ingressEngine := ingress.NewEngine(ingress.EngineOptions{
		Root:     filepath.Join(*root, ingress.DefaultIngressRoot),
		TLSAddr:  *tlsAddr,
		HTTPAddr: *httpAddr,
	})

	dnsEngine := dns.NewDNSEngine(dns.DNSEngineOptions{
		Root:     filepath.Join(*root, dns.DefaultZoneRoot),
		Addr:     *dnsAddr,
		MMDBPath: *mmdbPath,
	})

	group, groupCtx := errgroup.WithContext(ctx)
	if *tlsAddr != "" || *httpAddr != "" {
		group.Go(func() error {
			err := ingressEngine.Listen(groupCtx)
			if groupCtx.Err() != nil || errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		})
	}
	if *dnsAddr != "" {
		group.Go(func() error {
			err := dnsEngine.Listen(groupCtx)
			if groupCtx.Err() != nil || errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		})
	}
	if *masterURL != "" {
		podIPValue := *podIP
		agentIDValue := *agentID
		group.Go(func() error {
			client, err := replication.Dial(groupCtx, *masterURL)
			if err != nil {
				return fmt.Errorf("dial master %q: %w", *masterURL, err)
			}
			defer client.Close()

			err = ag.Subscribe(groupCtx, client, podIPValue, agentIDValue)
			if groupCtx.Err() != nil || errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		})
	}

	log.Printf(
		"agent started root=%s tls-addr=%s http-addr=%s dns-addr=%s master-url=%s",
		*root,
		*tlsAddr,
		*httpAddr,
		*dnsAddr,
		*masterURL,
	)

	if err := group.Wait(); err != nil {
		return err
	}
	return nil
}
