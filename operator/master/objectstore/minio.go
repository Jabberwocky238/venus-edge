package objectstore

import (
	"context"
	"fmt"
	"io"
)

type MinIO struct{}

func NewMinIO() *MinIO {
	return &MinIO{}
}

func (s *MinIO) Put(_ context.Context, _ string, _ io.Reader) error {
	return fmt.Errorf("minio object store is not implemented")
}

func (s *MinIO) Get(_ context.Context, _ string, _ io.Writer) error {
	return fmt.Errorf("minio object store is not implemented")
}
