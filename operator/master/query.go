package master

import (
	"bytes"
	"context"

	"aaa/operator/replication"
)

func (m *Master) readObject(ctx context.Context, kind replication.EventType, hostname string) ([]byte, error) {
	key, err := objectKey(kind, hostname)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := m.store.Get(ctx, key, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
