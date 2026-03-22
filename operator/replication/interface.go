package replication

import "context"

type ReplicationVersionMaintainer interface {
	Incre(ctx context.Context, event) error
}
