package replication

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type memoryVersionStore struct {
	version uint64
}

func (s *memoryVersionStore) Load(context.Context) (uint64, error) { return s.version, nil }
func (s *memoryVersionStore) Save(_ context.Context, version uint64) error {
	s.version = version
	return nil
}

type orderedApplier struct {
	applied []uint64
}

func (a *orderedApplier) Apply(_ context.Context, change *ChangeEnvelope) error {
	a.applied = append(a.applied, change.GetVersionIndex())
	return nil
}

type fakeSubscribeClient struct {
	streams []ReplicationService_SubscribeClient
	calls   []uint64
}

func (c *fakeSubscribeClient) Subscribe(_ context.Context, _, _ string, versionIndex uint64, _ ...grpc.CallOption) (ReplicationService_SubscribeClient, error) {
	c.calls = append(c.calls, versionIndex)
	if len(c.streams) == 0 {
		return nil, io.EOF
	}
	stream := c.streams[0]
	c.streams = c.streams[1:]
	return stream, nil
}

type fakeStream struct {
	changes []*ChangeEnvelope
	index   int
}

func (s *fakeStream) Header() (metadata.MD, error) { return nil, nil }
func (s *fakeStream) Trailer() metadata.MD         { return nil }
func (s *fakeStream) CloseSend() error             { return nil }
func (s *fakeStream) Context() context.Context     { return context.Background() }
func (s *fakeStream) SendMsg(any) error            { return nil }
func (s *fakeStream) RecvMsg(any) error            { return nil }
func (s *fakeStream) Recv() (*ChangeEnvelope, error) {
	if s.index >= len(s.changes) {
		return nil, io.EOF
	}
	change := s.changes[s.index]
	s.index++
	return cloneEnvelope(change), nil
}

func TestFollowerRecoverFlowAndReconnection(t *testing.T) {
	store := &memoryVersionStore{}
	applier := &orderedApplier{}
	client := &fakeSubscribeClient{
		streams: []ReplicationService_SubscribeClient{
			&fakeStream{changes: []*ChangeEnvelope{
				{VersionIndex: 5, Tier: MessageTier_MESSAGE_TIER_RECOVER, Hostname: "a", Bin: []byte("a")},
				{VersionIndex: 6, Tier: MessageTier_MESSAGE_TIER_NORMAL, Hostname: "b", Bin: []byte("b")},
				{VersionIndex: 7, Tier: MessageTier_MESSAGE_TIER_RECOVER, Hostname: "c", Bin: []byte("c")},
			}},
			&fakeStream{changes: []*ChangeEnvelope{
				{VersionIndex: 7, Tier: MessageTier_MESSAGE_TIER_RECOVER, Hostname: "c", Bin: []byte("c")},
				{VersionIndex: 8, Tier: MessageTier_MESSAGE_TIER_NORMAL, Hostname: "d", Bin: []byte("d")},
			}},
		},
	}

	follower, err := NewFollower(client, applier, store, "127.0.0.1", "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	follower.retry = 10 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Millisecond)
	defer cancel()
	err = follower.Run(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline, got %v", err)
	}
	if store.version != 8 {
		t.Fatalf("expected saved version 8, got %d", store.version)
	}
	if len(client.calls) < 2 || client.calls[0] != 0 || client.calls[1] != 6 {
		t.Fatalf("unexpected subscribe cursors: %#v", client.calls)
	}
}

func TestWALManagerRecoversLatestAppliedIndex(t *testing.T) {
	dir := t.TempDir()
	store := newMemoryChangeStore()
	wal, err := NewFileWAL(filepath.Join(dir, "agent", "wal"), store)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewWALManager("default", wal, store)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SaveApplied(context.Background(), &ChangeEnvelope{
		Type:         EventType_EVENT_TYPE_DNS,
		Hostname:     "app.com",
		Bin:          []byte("v7"),
		VersionIndex: 7,
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.SaveApplied(context.Background(), &ChangeEnvelope{
		Type:         EventType_EVENT_TYPE_DNS,
		Hostname:     "app.com",
		Bin:          []byte("v8"),
		VersionIndex: 8,
	}); err != nil {
		t.Fatal(err)
	}

	wal2, err := NewFileWAL(filepath.Join(dir, "agent", "wal"), store)
	if err != nil {
		t.Fatal(err)
	}
	manager2, err := NewWALManager("default", wal2, store)
	if err != nil {
		t.Fatal(err)
	}
	latest, err := manager2.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if latest != 8 {
		t.Fatalf("expected recovered latest index 8, got %d", latest)
	}
	if _, err := os.Stat(filepath.Join(dir, "agent", "wal", "wal.bin.1")); err != nil {
		t.Fatalf("expected agent wal segment file: %v", err)
	}
}

func TestFollowerUsesWALManagerForAppliedVersionRecovery(t *testing.T) {
	dir := t.TempDir()
	store := newMemoryChangeStore()
	wal, err := NewFileWAL(filepath.Join(dir, "agent", "wal"), store)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewWALManager("default", wal, store)
	if err != nil {
		t.Fatal(err)
	}
	applier := &orderedApplier{}
	client := &fakeSubscribeClient{
		streams: []ReplicationService_SubscribeClient{
			&fakeStream{changes: []*ChangeEnvelope{
				{VersionIndex: 3, Tier: MessageTier_MESSAGE_TIER_RECOVER, Hostname: "a", Bin: []byte("a")},
				{VersionIndex: 4, Tier: MessageTier_MESSAGE_TIER_NORMAL, Hostname: "b", Bin: []byte("b")},
			}},
		},
	}
	follower, err := NewFollower(client, applier, manager, "127.0.0.1", "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	follower.retry = 10 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err = follower.Run(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline, got %v", err)
	}

	wal2, err := NewFileWAL(filepath.Join(dir, "agent", "wal"), store)
	if err != nil {
		t.Fatal(err)
	}
	manager2, err := NewWALManager("default", wal2, store)
	if err != nil {
		t.Fatal(err)
	}
	latest, err := manager2.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if latest != 4 {
		t.Fatalf("expected recovered latest index 4 from agent wal, got %d", latest)
	}
}
