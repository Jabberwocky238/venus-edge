package master

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"aaa/operator/replication"
)

const (
	defaultMasterRoot = "."
	walDirName        = ".venus-edge/master"
	walFileCount      = 7
	walRotateInterval = 24 * time.Hour
)

type clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now()
}

type WALRecord struct {
	Hostname      string `json:"hostname"`
	ChangeType    string `json:"change_type"`
	ObjectPrefix  string `json:"object_prefix"`
	TimestampUnix int64  `json:"timestamp_unix"`
}

type WAL struct {
	mu        sync.Mutex
	dir       string
	file      *os.File
	index     int
	rotatedAt time.Time
	clock     clock
}

type WALStatus struct {
	Dir       string `json:"dir"`
	ActiveFile string `json:"active_file"`
	Index     int    `json:"index"`
	RotatedAt int64  `json:"rotated_at"`
}

func NewWAL(root string) (*WAL, error) {
	if root == "" {
		root = defaultMasterRoot
	}
	dir := filepath.Join(root, walDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	w := &WAL{
		dir:   dir,
		index: 1,
		clock: realClock{},
	}
	if err := w.openCurrent(true); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *WAL) Append(record WALRecord) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.rotateIfNeeded(); err != nil {
		return err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if _, err := w.file.Write(append(payload, '\n')); err != nil {
		return err
	}
	return w.file.Sync()
}

func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *WAL) rotateIfNeeded() error {
	if w.file == nil {
		return w.openCurrent(true)
	}
	if w.clock.Now().Sub(w.rotatedAt) < walRotateInterval {
		return nil
	}
	if err := w.file.Close(); err != nil {
		return err
	}
	w.file = nil
	w.index++
	if w.index > walFileCount {
		w.index = 1
	}
	return w.openCurrent(true)
}

func (w *WAL) openCurrent(truncate bool) error {
	flags := os.O_CREATE | os.O_WRONLY
	if truncate {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_APPEND
	}
	path := filepath.Join(w.dir, walFileName(w.index))
	file, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return err
	}
	w.file = file
	w.rotatedAt = w.clock.Now()
	return nil
}

func walFileName(index int) string {
	return fmt.Sprintf("wal.%d", index)
}

func newWALRecord(hostname, prefix string, kind replication.EventType, ts int64) WALRecord {
	return WALRecord{
		Hostname:      hostname,
		ChangeType:    kind.String(),
		ObjectPrefix:  prefix,
		TimestampUnix: ts,
	}
}

func (w *WAL) Status() WALStatus {
	w.mu.Lock()
	defer w.mu.Unlock()

	return WALStatus{
		Dir:        w.dir,
		ActiveFile: filepath.Join(w.dir, walFileName(w.index)),
		Index:      w.index,
		RotatedAt:  w.rotatedAt.Unix(),
	}
}

func (w *WAL) Files() []string {
	w.mu.Lock()
	defer w.mu.Unlock()

	files := make([]string, 0, walFileCount)
	for i := 1; i <= walFileCount; i++ {
		files = append(files, filepath.Join(w.dir, walFileName(i)))
	}
	return files
}
