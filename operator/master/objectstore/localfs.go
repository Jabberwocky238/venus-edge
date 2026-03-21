package objectstore

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

type LocalFS struct {
	root string
}

func NewLocalFS(root string) *LocalFS {
	return &LocalFS{root: root}
}

func (s *LocalFS) Put(_ context.Context, key string, r io.Reader) error {
	path := filepath.Join(s.root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

func (s *LocalFS) Get(_ context.Context, key string, w io.Writer) error {
	path := filepath.Join(s.root, filepath.FromSlash(key))
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}
