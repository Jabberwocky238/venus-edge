package replication

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
)

type SubscribeClient interface {
	Subscribe(ctx context.Context, podIP, agentID string, versionIndex uint64, opts ...grpc.CallOption) (ReplicationService_SubscribeClient, error)
}

type ChangeApplier interface {
	Apply(context.Context, *ChangeEnvelope) error
}

type VersionStore interface {
	Load(context.Context) (uint64, error)
	Save(context.Context, uint64) error
}

type Follower struct {
	client  SubscribeClient
	applier ChangeApplier
	store   VersionStore
	podIP   string
	agentID string
	retry   time.Duration
}

func NewFollower(client SubscribeClient, applier ChangeApplier, store VersionStore, podIP, agentID string) (*Follower, error) {
	if client == nil {
		return nil, fmt.Errorf("subscribe client is required")
	}
	if applier == nil {
		return nil, fmt.Errorf("change applier is required")
	}
	if store == nil {
		return nil, fmt.Errorf("version store is required")
	}
	if podIP == "" {
		return nil, fmt.Errorf("pod_ip is required")
	}
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	return &Follower{
		client:  client,
		applier: applier,
		store:   store,
		podIP:   podIP,
		agentID: agentID,
		retry:   300 * time.Millisecond,
	}, nil
}

func (f *Follower) Run(ctx context.Context) error {
	for {
		current, err := f.store.Load(ctx)
		if err != nil {
			return err
		}

		stream, err := f.client.Subscribe(ctx, f.podIP, f.agentID, current)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if waitErr := sleepContext(ctx, f.retry); waitErr != nil {
				return waitErr
			}
			continue
		}

		err = f.consume(ctx, stream, current)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil || errors.Is(err, io.EOF) || errors.Is(err, ErrFutureVersion) || errors.Is(err, ErrVersionGap) || errors.Is(err, ErrRecoverAfterNormal) {
			if waitErr := sleepContext(ctx, f.retry); waitErr != nil {
				return waitErr
			}
			continue
		}
		return err
	}
}

var ErrVersionGap = errors.New("received version gap from replication stream")
var ErrRecoverAfterNormal = errors.New("received recover tier after stream entered normal mode")

func (f *Follower) consume(ctx context.Context, stream ReplicationService_SubscribeClient, current uint64) error {
	recovering := true
	for {
		change, err := stream.Recv()
		if err != nil {
			return err
		}
		if change == nil {
			continue
		}

		tier := change.GetTier()
		switch tier {
		case MessageTier_MESSAGE_TIER_RECOVER:
			if !recovering {
				return fmt.Errorf("%w: current=%d got=%d", ErrRecoverAfterNormal, current, change.GetVersionIndex())
			}
		case MessageTier_MESSAGE_TIER_NORMAL:
			recovering = false
		default:
			recovering = false
		}

		switch {
		case change.GetVersionIndex() == 0:
			return fmt.Errorf("replication stream returned empty version index")
		case tier == MessageTier_MESSAGE_TIER_RECOVER:
			if change.GetVersionIndex() <= current {
				continue
			}
		default:
			expected := current + 1
			if change.GetVersionIndex() < expected {
				continue
			}
			if change.GetVersionIndex() > expected {
				return fmt.Errorf("%w: have=%d got=%d", ErrVersionGap, current, change.GetVersionIndex())
			}
		}

		if err := f.applier.Apply(ctx, change); err != nil {
			return err
		}
		current = change.GetVersionIndex()
		if err := f.store.Save(ctx, current); err != nil {
			return err
		}
	}
}

type FileVersionStore struct {
	path string
	mu   sync.Mutex
}

func NewFileVersionStore(path string) *FileVersionStore {
	return &FileVersionStore{path: path}
}

func (s *FileVersionStore) Load(_ context.Context) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return 0, nil
	}
	return strconv.ParseUint(value, 10, 64)
}

func (s *FileVersionStore) Save(_ context.Context, version uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strconv.FormatUint(version, 10)), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
