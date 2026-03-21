package objectstore

import (
	"context"
	"io"
)

type Store interface {
	Put(ctx context.Context, key string, r io.Reader) error
	Get(ctx context.Context, key string, w io.Writer) error
}
