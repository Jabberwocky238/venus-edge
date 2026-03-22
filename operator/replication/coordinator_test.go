package replication

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/grpc/metadata"
)

type memoryChangeStore struct {
	data map[string][]byte
}

func newMemoryChangeStore() *memoryChangeStore {
	return &memoryChangeStore{data: make(map[string][]byte)}
}

func (s *memoryChangeStore) Persist(_ context.Context, change *ChangeEnvelope) error {
	s.data[s.key(change.GetType(), change.GetHostname())] = append([]byte(nil), change.GetBin()...)
	return nil
}

func (s *memoryChangeStore) Load(_ context.Context, kind EventType, hostname string) ([]byte, error) {
	data, ok := s.data[s.key(kind, hostname)]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

func (s *memoryChangeStore) key(kind EventType, hostname string) string {
	return kind.String() + ":" + hostname
}

type recordingSubscribeStream struct {
	ctx    context.Context
	cancel context.CancelFunc
	sent   []*ChangeEnvelope
	limit  int
}

func newRecordingSubscribeStream(limit int) *recordingSubscribeStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &recordingSubscribeStream{ctx: ctx, cancel: cancel, limit: limit}
}

func (s *recordingSubscribeStream) Send(change *ChangeEnvelope) error {
	s.sent = append(s.sent, cloneEnvelope(change))
	if s.limit > 0 && len(s.sent) >= s.limit {
		s.cancel()
		return io.EOF
	}
	return nil
}

func (s *recordingSubscribeStream) SetHeader(metadata.MD) error  { return nil }
func (s *recordingSubscribeStream) SendHeader(metadata.MD) error { return nil }
func (s *recordingSubscribeStream) SetTrailer(metadata.MD)       {}
func (s *recordingSubscribeStream) Context() context.Context     { return s.ctx }
func (s *recordingSubscribeStream) SendMsg(any) error            { return nil }
func (s *recordingSubscribeStream) RecvMsg(any) error            { return nil }

func TestFileWALOverlapAndRecoverReplay(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := newMemoryChangeStore()
	wal, err := NewFileWAL(dir, store)
	if err != nil {
		t.Fatal(err)
	}
	coord, err := NewCoordinator("default", wal)
	if err != nil {
		t.Fatal(err)
	}

	v1, err := coord.Publish(ctx, EventType_EVENT_TYPE_DNS, "app.com", []byte("v1"))
	if err != nil {
		t.Fatal(err)
	}
	v2, err := coord.Publish(ctx, EventType_EVENT_TYPE_DNS, "app.com", []byte("v2"))
	if err != nil {
		t.Fatal(err)
	}
	v3, err := coord.Publish(ctx, EventType_EVENT_TYPE_TLS, "app.com", []byte("tls"))
	if err != nil {
		t.Fatal(err)
	}
	if v1 != 1 || v2 != 2 || v3 != 3 {
		t.Fatalf("unexpected versions: %d %d %d", v1, v2, v3)
	}

	_, logRoot, err := wal.readSegmentLocked(1)
	if err != nil {
		t.Fatal(err)
	}
	items, err := logRoot.Items()
	if err != nil {
		t.Fatal(err)
	}
	if items.Len() != 3 {
		t.Fatalf("expected 3 wal items, got %d", items.Len())
	}
	if items.At(0).Status() != EventItem_Status_overlaped {
		t.Fatalf("expected first item overlaped, got %v", items.At(0).Status())
	}
	if items.At(1).Status() != EventItem_Status_ontop {
		t.Fatalf("expected second item ontop, got %v", items.At(1).Status())
	}
	if items.At(1).LastAffectIndex() != 1 {
		t.Fatalf("expected lastAffectIndex=1, got %d", items.At(1).LastAffectIndex())
	}
	if items.At(2).Status() != EventItem_Status_ontop {
		t.Fatalf("expected third item ontop, got %v", items.At(2).Status())
	}
	if logRoot.NotOverlap() != 2 {
		t.Fatalf("expected notOverlap=2, got %d", logRoot.NotOverlap())
	}

	stream := newRecordingSubscribeStream(2)
	err = coord.HandleSubscribe(&PushChangeRequest{
		PodIp:        "127.0.0.1",
		AgentId:      "agent-1",
		VersionIndex: 0,
	}, stream)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if len(stream.sent) != 2 {
		t.Fatalf("expected 2 recover envelopes, got %d", len(stream.sent))
	}
	if stream.sent[0].GetVersionIndex() != 2 || stream.sent[1].GetVersionIndex() != 3 {
		t.Fatalf("unexpected recover order: %d %d", stream.sent[0].GetVersionIndex(), stream.sent[1].GetVersionIndex())
	}
	if stream.sent[0].GetTier() != MessageTier_MESSAGE_TIER_RECOVER || stream.sent[1].GetTier() != MessageTier_MESSAGE_TIER_RECOVER {
		t.Fatalf("expected recover tier for replay")
	}
	if string(stream.sent[0].GetBin()) != "v2" || string(stream.sent[1].GetBin()) != "tls" {
		t.Fatalf("unexpected replay bin payloads")
	}
}

func TestFileWALSegmentsEveryThousandItems(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := newMemoryChangeStore()
	wal, err := NewFileWAL(dir, store)
	if err != nil {
		t.Fatal(err)
	}
	coord, err := NewCoordinator("default", wal)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 1001; i++ {
		hostname := "host-" + strconvI(i) + ".example.com"
		if _, err := coord.Publish(ctx, EventType_EVENT_TYPE_HTTP, hostname, []byte(hostname)); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := os.Stat(filepath.Join(dir, "wal.bin.1")); err != nil {
		t.Fatalf("expected wal.bin.1: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "wal.bin.2")); err != nil {
		t.Fatalf("expected wal.bin.2: %v", err)
	}

	backlog, err := wal.Since(ctx, 999)
	if err != nil {
		t.Fatal(err)
	}
	if len(backlog) != 2 {
		t.Fatalf("expected 2 recover items after cursor 999, got %d", len(backlog))
	}
	if backlog[0].GetVersionIndex() != 1000 || backlog[1].GetVersionIndex() != 1001 {
		t.Fatalf("unexpected cross-segment replay order: %d %d", backlog[0].GetVersionIndex(), backlog[1].GetVersionIndex())
	}
}

func strconvI(v int) string {
	return fmt.Sprintf("%d", v)
}
