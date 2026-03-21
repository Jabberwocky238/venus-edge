package master

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	dns "aaa/DNS"
	ingress "aaa/ingress"
	"aaa/operator/replication"
)

type ObjectStore interface {
	Put(ctx context.Context, key string, value []byte) error
}

type Publisher interface {
	Publish(change *replication.ChangeEnvelope) *replication.PushChangeResponse
}

type Master struct {
	store     ObjectStore
	publisher Publisher
	wal       *WAL
}

func New(store ObjectStore, publisher Publisher) *Master {
	return mustNew(defaultMasterRoot, store, publisher)
}

func NewWithRoot(root string, store ObjectStore, publisher Publisher) (*Master, error) {
	return newMaster(root, store, publisher)
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
	if m.store == nil || m.publisher == nil {
		return nil, fmt.Errorf("master is not configured")
	}
	key, err := objectKey(kind, hostname)
	if err != nil {
		return nil, err
	}
	if err := m.store.Put(ctx, key, bin); err != nil {
		return nil, err
	}
	ts := time.Now().Unix()
	if m.wal != nil {
		if err := m.wal.Append(newWALRecord(hostname, key, kind, ts)); err != nil {
			return nil, err
		}
	}
	return m.publisher.Publish(&replication.ChangeEnvelope{
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

func newMaster(root string, store ObjectStore, publisher Publisher) (*Master, error) {
	wal, err := NewWAL(root)
	if err != nil {
		return nil, err
	}
	return &Master{
		store:     store,
		publisher: publisher,
		wal:       wal,
	}, nil
}

func mustNew(root string, store ObjectStore, publisher Publisher) *Master {
	m, err := newMaster(root, store, publisher)
	if err != nil {
		panic(err)
	}
	return m
}

func (m *Master) WALStatus() WALStatus {
	if m == nil || m.wal == nil {
		return WALStatus{}
	}
	return m.wal.Status()
}

func (m *Master) WALFiles() []string {
	if m == nil || m.wal == nil {
		return nil
	}
	return m.wal.Files()
}
