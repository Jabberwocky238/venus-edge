package master

import (
	"bytes"
	"context"

	"aaa/operator/master/objectstore"
	"aaa/operator/replication"
)

type storePersistor struct {
	store objectstore.Store
}

func (p storePersistor) Persist(ctx context.Context, change *replication.ChangeEnvelope) error {
	key, err := objectKey(change.GetType(), change.GetHostname())
	if err != nil {
		return err
	}
	return p.store.Put(ctx, key, bytes.NewReader(change.GetBin()))
}

func (p storePersistor) Load(ctx context.Context, kind replication.EventType, hostname string) ([]byte, error) {
	key, err := objectKey(kind, hostname)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := p.store.Get(ctx, key, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
