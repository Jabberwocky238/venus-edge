package master

import (
	"bytes"
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

type Options struct {
	Root       string
	Store      objectstore.Store
	ManageAddr string
	GRPCAddr   string
	WebRoot    string
}

type Master struct {
	root       string
	store      objectstore.Store
	hub        *Hub
	manageAddr string
	grpcAddr   string
	webRoot    string
}

func New(opts Options) (*Master, error) {
	root := opts.Root
	if root == "" {
		root = defaultMasterRoot
	}
	if opts.Store == nil {
		return nil, fmt.Errorf("store is required")
	}
	manageAddr := opts.ManageAddr
	if manageAddr == "" {
		manageAddr = ":9000"
	}
	grpcAddr := opts.GRPCAddr
	if grpcAddr == "" {
		grpcAddr = ":10992"
	}
	return &Master{
		root:       root,
		store:      opts.Store,
		hub:        NewHub(),
		manageAddr: manageAddr,
		grpcAddr:   grpcAddr,
		webRoot:    opts.WebRoot,
	}, nil
}

func (m *Master) PublishDNS(ctx context.Context, hostname string, bin []byte) (*replication.PushChangeResponse, error) {
	return m.publish(ctx, replication.EventType_EVENT_TYPE_DNS, hostname, bin)
}

func (m *Master) PublishTLS(ctx context.Context, hostname string, bin []byte) (*replication.PushChangeResponse, error) {
	return m.publish(ctx, replication.EventType_EVENT_TYPE_TLS, hostname, bin)
}

func (m *Master) PublishHTTP(ctx context.Context, hostname string, bin []byte) (*replication.PushChangeResponse, error) {
	return m.publish(ctx, replication.EventType_EVENT_TYPE_HTTP, hostname, bin)
}

func (m *Master) publish(ctx context.Context, kind replication.EventType, hostname string, bin []byte) (*replication.PushChangeResponse, error) {
	if m.store == nil || m.hub == nil {
		return nil, fmt.Errorf("master is not configured")
	}
	key, err := objectKey(kind, hostname)
	if err != nil {
		return nil, err
	}
	if err := m.store.Put(ctx, key, bytes.NewReader(bin)); err != nil {
		return nil, err
	}
	ts := envelopeTimestampUnix()
	return m.hub.Publish(&replication.ChangeEnvelope{
		Cluster:       "default",
		Type:          kind,
		Hostname:      hostname,
		Bin:           bin,
		TimestampUnix: ts,
	}), nil
}

func objectKey(kind replication.EventType, hostname string) (string, error) {
	switch kind {
	case replication.EventType_EVENT_TYPE_DNS:
		return filepath.ToSlash(filepath.Join(dns.DefaultZoneDir, hostname+".bin")), nil
	case replication.EventType_EVENT_TYPE_TLS:
		return filepath.ToSlash(filepath.Join(ingress.DefaultTLSDir, hostname+".bin")), nil
	case replication.EventType_EVENT_TYPE_HTTP:
		return filepath.ToSlash(filepath.Join(ingress.DefaultHTTPDir, hostname+".bin")), nil
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

	manageServer, err := NewManageServer(m, m.hub, m.webRoot)
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
	replication.NewServer(m.hub).Register(grpcServer)
	defer grpcServer.GracefulStop()

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		log.Printf("master manage api listening on %s", m.manageAddr)
		err := httpServer.ListenAndServe()
		if groupCtx.Err() != nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	})
	group.Go(func() error {
		log.Printf("master grpc listening on %s", m.grpcAddr)
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
