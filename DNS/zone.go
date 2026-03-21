package dns

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	capnp "capnproto.org/go/capnp/v3"
)

const (
	DefaultZoneRoot = ".venus-edge"
	DefaultZoneDir  = "dns"
)

type ZoneStore interface {
	OpenZone(zone string) (io.ReadCloser, error)
}

type FSStore struct {
	Root string
}

type RecordBuilder interface {
	Type() RecordType
	Build(record DnsRecord) error
}

func EnsureZoneDir(root string) error {
	return os.MkdirAll(filepath.Join(root, DefaultZoneDir), 0o755)
}

func NewFSStore(root string) (FSStore, error) {
	if err := EnsureZoneDir(root); err != nil {
		return FSStore{}, err
	}
	return FSStore{Root: root}, nil
}

func (s FSStore) OpenZone(zone string) (io.ReadCloser, error) {
	return OpenZoneFile(s.Root, zone)
}

func (s FSStore) Read(zone string) (Zone, error) {
	f, err := s.OpenZone(zone)
	if err != nil {
		return Zone{}, err
	}
	defer f.Close()
	return Read(f)
}

func (s FSStore) Write(zone string, records ...RecordBuilder) error {
	return Write(ZoneFilePath(s.Root, zone), records...)
}

func OpenZoneFile(root, zone string) (io.ReadCloser, error) {
	path := ZoneFilePath(root, zone)
	return retryOpenFile(path)
}

func ZoneFilePath(root, zone string) string {
	return filepath.Join(root, DefaultZoneDir, sanitizeZoneName(zone)+".bin")
}

func CandidateZones(name string) []string {
	trimmed := strings.TrimSuffix(normalizeName(name), ".")
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, ".")
	candidates := make([]string, 0, len(parts))
	for i := 0; i < len(parts); i++ {
		candidates = append(candidates, strings.Join(parts[i:], "."))
	}
	return candidates
}

func Read(r io.Reader) (Zone, error) {
	msg, err := capnp.NewDecoder(r).Decode()
	if err != nil {
		return Zone{}, fmt.Errorf("decode zone: %w", err)
	}
	zone, err := ReadRootZone(msg)
	if err != nil {
		return Zone{}, fmt.Errorf("read root zone: %w", err)
	}
	return zone, nil
}

func Write(path string, records ...RecordBuilder) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create zone dir: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create zone file: %w", err)
	}
	defer f.Close()
	return writeTo(f, records...)
}

func writeTo(w io.Writer, records ...RecordBuilder) error {
	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return fmt.Errorf("new capnp message: %w", err)
	}
	zone, err := NewRootZone(seg)
	if err != nil {
		return fmt.Errorf("new root zone: %w", err)
	}
	list, err := zone.NewRecords(int32(len(records)))
	if err != nil {
		return fmt.Errorf("new records list: %w", err)
	}
	indexes := map[RecordType][]uint32{}
	for i, builder := range records {
		record := list.At(i)
		if err := builder.Build(record); err != nil {
			return fmt.Errorf("build record %d: %w", i, err)
		}
		indexes[builder.Type()] = append(indexes[builder.Type()], uint32(i))
	}
	if err := setZoneIndexes(zone, indexes); err != nil {
		return err
	}
	if err := capnp.NewEncoder(w).Encode(msg); err != nil {
		return fmt.Errorf("encode zone: %w", err)
	}
	return nil
}

func setZoneIndexes(zone Zone, indexes map[RecordType][]uint32) error {
	for _, spec := range []struct {
		kind    RecordType
		newList func(int32) (capnp.UInt32List, error)
	}{
		{RecordType_a, zone.NewAIndexes},
		{RecordType_aaaa, zone.NewAaaaIndexes},
		{RecordType_cname, zone.NewCnameIndexes},
		{RecordType_mx, zone.NewMxIndexes},
		{RecordType_ns, zone.NewNsIndexes},
		{RecordType_ptr, zone.NewPtrIndexes},
		{RecordType_soa, zone.NewSoaIndexes},
		{RecordType_txt, zone.NewTxtIndexes},
	} {
		values := indexes[spec.kind]
		list, err := spec.newList(int32(len(values)))
		if err != nil {
			return fmt.Errorf("new %s indexes: %w", spec.kind.String(), err)
		}
		for i, v := range values {
			list.Set(i, v)
		}
	}
	return nil
}

func sanitizeZoneName(zone string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(zone), "."))
}

func retryOpenFile(path string) (io.ReadCloser, error) {
	delays := []time.Duration{0, 10 * time.Millisecond, 30 * time.Millisecond, 80 * time.Millisecond}
	var lastErr error
	for _, delay := range delays {
		if delay > 0 {
			time.Sleep(delay)
		}
		f, err := os.Open(path)
		if err == nil {
			return f, nil
		}
		if !shouldRetryFileRead(err) {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}

func shouldRetryFileRead(err error) bool {
	if err == nil || os.IsNotExist(err) {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "used by another process") ||
		strings.Contains(text, "file is being used by another process") ||
		strings.Contains(text, "sharing violation") ||
		strings.Contains(text, "permission denied") ||
		strings.Contains(text, "resource temporarily unavailable") ||
		strings.Contains(text, "temporarily unavailable") ||
		strings.Contains(text, "input/output error")
}
