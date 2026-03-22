package master

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"time"

	dns "aaa/DNS"
	ingress "aaa/ingress"
	"aaa/operator/master/objectstore"
	"aaa/operator/replication"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

const defaultMasterRoot = "."

const (
	masterLogPrefix = "\033[38;5;220m[MASTER]\033[0m"
	masterLogOK     = "\033[32m"
	masterLogFail   = "\033[31m"
	masterLogReset  = "\033[0m"
	masterDNSLabel  = "\033[38;5;117m[DNS]\033[0m"
	masterHTTPLabel = "\033[38;5;229m[HTTP]\033[0m"
	masterTLSLabel  = "\033[38;5;183m[TLS]\033[0m"
)

type Options struct {
	Root       string
	Store      objectstore.Store
	ManageAddr string
	GRPCAddr   string
	WebRoot    string
	ACME       ACMEConfig
}

type ACMEConfig struct {
	DefaultProvider string
	DefaultEmail    string
	ZeroSSLEABKID   string
	ZeroSSLEABHMAC  string
}

type Master struct {
	root        string
	store       objectstore.Store
	replication *replication.Coordinator
	manageAddr  string
	grpcAddr    string
	webRoot     string
	acme        ACMEConfig
}

func New(opts Options) (*Master, error) {
	if opts.Store == nil {
		return nil, fmt.Errorf("store is required")
	}
	storeAdapter := storePersistor{store: opts.Store}
	wal, err := replication.NewFileWAL(filepath.Join(opts.Root, "operator", "master", "wal"), storeAdapter)
	if err != nil {
		return nil, err
	}
	coord, err := replication.NewCoordinator("default", wal)
	if err != nil {
		return nil, err
	}
	return &Master{
		root:        opts.Root,
		store:       opts.Store,
		replication: coord,
		manageAddr:  opts.ManageAddr,
		grpcAddr:    opts.GRPCAddr,
		webRoot:     opts.WebRoot,
		acme:        opts.ACME,
	}, nil
}

func (m *Master) ACME() ACMEConfig {
	if m == nil {
		return ACMEConfig{}
	}
	return m.acme
}

func (m *Master) PublishDNS(ctx context.Context, hostname string, bin []byte) (version uint64, err error) {
	return m.publish(ctx, replication.EventType_EVENT_TYPE_DNS, hostname, bin)
}

func (m *Master) PublishTLS(ctx context.Context, hostname string, bin []byte) (version uint64, err error) {
	return m.publish(ctx, replication.EventType_EVENT_TYPE_TLS, hostname, bin)
}

func (m *Master) PublishHTTP(ctx context.Context, hostname string, bin []byte) (version uint64, err error) {
	return m.publish(ctx, replication.EventType_EVENT_TYPE_HTTP, hostname, bin)
}

func (m *Master) publish(ctx context.Context, kind replication.EventType, hostname string, bin []byte) (uint64, error) {
	if m.store == nil || m.replication == nil {
		return 0, fmt.Errorf("master is not configured")
	}
	key, err := objectKey(kind, hostname)
	if err != nil {
		logMasterPublish(kind, hostname, "", 0, err)
		return 0, err
	}
	version, err := m.replication.Publish(ctx, kind, hostname, bin)
	if err != nil {
		logMasterPublish(kind, hostname, key, 0, err)
		return 0, err
	}
	logMasterPublish(kind, hostname, key, version, nil)
	return version, nil
}

func objectKey(kind replication.EventType, hostname string) (string, error) {
	switch kind {
	case replication.EventType_EVENT_TYPE_DNS:
		return filepath.ToSlash(filepath.Join("dns", filepath.Base(dns.ZoneFilePath("", hostname)))), nil
	case replication.EventType_EVENT_TYPE_TLS:
		return filepath.ToSlash(filepath.Join("tls", filepath.Base(ingress.TLSZoneFilePath("", hostname)))), nil
	case replication.EventType_EVENT_TYPE_HTTP:
		return filepath.ToSlash(filepath.Join("http", filepath.Base(ingress.HTTPZoneFilePath("", hostname)))), nil
	default:
		return "", fmt.Errorf("unsupported event type: %v", kind)
	}
}

func (m *Master) Start(ctx context.Context) error {
	if m == nil {
		return fmt.Errorf("master is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	manageServer, err := NewManageServer(m, m.webRoot)
	if err != nil {
		return err
	}
	httpServer := &http.Server{
		Addr:              m.manageAddr,
		Handler:           manageServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	grpcListener, err := net.Listen("tcp", m.grpcAddr)
	if err != nil {
		return err
	}
	defer grpcListener.Close()

	grpcServer := grpc.NewServer()
	replication.NewServer(m.replication).Register(grpcServer)
	defer grpcServer.GracefulStop()

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		log.Printf("master manage api listening on http://%s", m.manageAddr)
		err := httpServer.ListenAndServe()
		if groupCtx.Err() != nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	})
	group.Go(func() error {
		log.Printf("master grpc listening on grpc://%s", m.grpcAddr)
		err := grpcServer.Serve(grpcListener)
		if groupCtx.Err() != nil || errors.Is(err, net.ErrClosed) {
			return nil
		}
		return err
	})
	group.Go(func() error {
		<-groupCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		grpcServer.GracefulStop()
		return nil
	})

	return group.Wait()
}

func logMasterPublish(kind replication.EventType, hostname, key string, version uint64, err error) {
	if err != nil {
		log.Printf("%s %spublish%s %s hostname=%s key=%s err=%v", masterLogPrefix, masterLogFail, masterLogReset, masterEventLabel(kind), hostname, key, err)
		return
	}
	log.Printf("%s %spublish%s %s hostname=%s key=%s version_index=%d", masterLogPrefix, masterLogOK, masterLogReset, masterEventLabel(kind), hostname, key, version)
}

func masterEventLabel(kind replication.EventType) string {
	switch kind {
	case replication.EventType_EVENT_TYPE_DNS:
		return masterDNSLabel
	case replication.EventType_EVENT_TYPE_TLS:
		return masterTLSLabel
	case replication.EventType_EVENT_TYPE_HTTP:
		return masterHTTPLabel
	default:
		return kind.String()
	}
}
