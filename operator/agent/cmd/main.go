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
	"strings"
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
		root       = flag.String("root", envOrDefault("VENUS_AGENT_ROOT", "."), "workspace root for agent data")
		tlsAddr    = flag.String("tls-addr", envOrDefault("VENUS_TLS_LISTEN", "127.0.0.1:8443"), "TLS engine listen address")
		httpAddr   = flag.String("http-addr", envOrDefault("VENUS_HTTP_LISTEN", "127.0.0.1:8080"), "HTTP engine listen address")
		dnsAddr    = flag.String("dns-addr", envOrDefault("VENUS_DNS_LISTEN", "127.0.0.1:8053"), "DNS engine listen address")
		healthAddr = flag.String("health-addr", envOrDefault("VENUS_HEALTH_LISTEN", ""), "agent health check listen address")
		masterURL  = flag.String("master-url", envOrDefault("VENUS_MASTER_ADDRESS", "127.0.0.1:10992"), "operator master replication endpoint")
		podIP      = flag.String("pod-ip", envOrDefault("VENUS_POD_IP", "127.0.0.1"), "subscriber pod IP for master replication")
		agentID    = flag.String("agent-id", envOrDefault("VENUS_AGENT_ID", "agent"), "subscriber agent ID for master replication")
		mmdbPath   = flag.String("mmdb-path", envOrDefault("VENUS_DNS_GEOIP_MMDB_PATH", "./data/GeoLite2-City.mmdb"), "optional GeoIP MMDB path for DNS engine")
		forwarders = flag.String("dns-forward-servers", envOrDefault("VENUS_DNS_FORWARD_SERVERS", "1.1.1.1:53,8.8.8.8:53"), "comma-separated upstream DNS forward servers")
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
		Root:           filepath.Join(*root, dns.DefaultZoneRoot),
		Addr:           *dnsAddr,
		MMDBPath:       *mmdbPath,
		ForwardServers: splitCSV(*forwarders),
	})

	group, groupCtx := errgroup.WithContext(ctx)
	if *healthAddr != "" {
		health := newHealthServer(*healthAddr, ingressEngine, dnsEngine, *tlsAddr, *httpAddr, *dnsAddr)
		group.Go(func() error {
			err := health.Listen(groupCtx)
			if groupCtx.Err() != nil || errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		})
	}
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
		"agent started root=%s tls-addr=%s http-addr=%s dns-addr=%s health-addr=%s master-url=%s forwarders=%s",
		*root,
		*tlsAddr,
		*httpAddr,
		*dnsAddr,
		*healthAddr,
		*masterURL,
		*forwarders,
	)

	if err := group.Wait(); err != nil {
		return err
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return []string{"1.1.1.1:53"}
	}
	return out
}
