package replication

import (
	"context"
	"fmt"
	"time"
)

type WALManager struct {
	cluster string
	wal     *FileWAL
	store   ChangeStore
}

func NewWALManager(cluster string, wal *FileWAL, store ChangeStore) (*WALManager, error) {
	if wal == nil {
		return nil, fmt.Errorf("wal is required")
	}
	if store == nil {
		return nil, fmt.Errorf("change store is required")
	}
	if cluster == "" {
		cluster = "default"
	}
	return &WALManager{
		cluster: cluster,
		wal:     wal,
		store:   store,
	}, nil
}

func (m *WALManager) Publish(ctx context.Context, kind EventType, hostname string, bin []byte) (*ChangeEnvelope, error) {
	change := &ChangeEnvelope{
		Cluster:       m.cluster,
		Type:          kind,
		Hostname:      hostname,
		Bin:           append([]byte(nil), bin...),
		TimestampUnix: time.Now().Unix(),
		Tier:          MessageTier_MESSAGE_TIER_NORMAL,
	}
	if err := m.store.Persist(ctx, change); err != nil {
		return nil, err
	}
	return m.wal.Append(ctx, change)
}

func (m *WALManager) PersistChange(ctx context.Context, change *ChangeEnvelope) error {
	return m.store.Persist(ctx, change)
}

func (m *WALManager) ReplaySince(ctx context.Context, version uint64) ([]*ChangeEnvelope, error) {
	return m.wal.Since(ctx, version)
}

func (m *WALManager) Latest(ctx context.Context) (uint64, error) {
	return m.wal.Latest(ctx)
}

func (m *WALManager) Load(ctx context.Context) (uint64, error) {
	return m.Latest(ctx)
}

func (m *WALManager) Save(context.Context, uint64) error {
	return fmt.Errorf("plain Save is not supported; use SaveApplied")
}

func (m *WALManager) SaveApplied(ctx context.Context, change *ChangeEnvelope) error {
	_, err := m.wal.AppendApplied(ctx, change)
	return err
}
