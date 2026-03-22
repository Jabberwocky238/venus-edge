package replication

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	capnp "capnproto.org/go/capnp/v3"
)

var ErrFutureVersion = errors.New("replication version is ahead of master")

const subscriberBuffer = 32
const walItemsPerFile = uint64(1000)

type ChangeStore interface {
	Persist(context.Context, *ChangeEnvelope) error
	Load(context.Context, EventType, string) ([]byte, error)
}

type WAL interface {
	Append(context.Context, *ChangeEnvelope) (*ChangeEnvelope, error)
	Since(context.Context, uint64) ([]*ChangeEnvelope, error)
	Latest(context.Context) (uint64, error)
}

type SubscriberSnapshot struct {
	PodIP   string `json:"pod_ip"`
	AgentID string `json:"agent_id"`
}

type Coordinator struct {
	cluster     string
	wal         WAL
	mu          sync.RWMutex
	subscribers map[string]*subscriber
}

type subscriber struct {
	key     string
	podIP   string
	agentID string
	ch      chan *ChangeEnvelope
}

func NewCoordinator(cluster string, wal WAL) (*Coordinator, error) {
	if wal == nil {
		return nil, fmt.Errorf("wal is required")
	}
	if cluster == "" {
		cluster = "default"
	}
	return &Coordinator{
		cluster:     cluster,
		wal:         wal,
		subscribers: make(map[string]*subscriber),
	}, nil
}

func (c *Coordinator) Publish(ctx context.Context, kind EventType, hostname string, bin []byte) (uint64, error) {
	if len(bin) == 0 {
		return 0, fmt.Errorf("bin is required")
	}
	if hostname == "" {
		return 0, fmt.Errorf("hostname is required")
	}

	change, err := c.wal.Append(ctx, &ChangeEnvelope{
		Cluster:       c.cluster,
		Type:          kind,
		Hostname:      hostname,
		Bin:           append([]byte(nil), bin...),
		TimestampUnix: time.Now().Unix(),
		Tier:          MessageTier_MESSAGE_TIER_NORMAL,
	})
	if err != nil {
		return 0, err
	}

	c.broadcast(change)
	return change.GetVersionIndex(), nil
}

func (c *Coordinator) HandleSubscribe(req *PushChangeRequest, stream ReplicationService_SubscribeServer) error {
	if req == nil {
		return fmt.Errorf("subscribe request is required")
	}

	latest, err := c.wal.Latest(stream.Context())
	if err != nil {
		return err
	}
	if req.GetVersionIndex() > latest {
		return fmt.Errorf("%w: agent=%d master=%d", ErrFutureVersion, req.GetVersionIndex(), latest)
	}

	backlog, err := c.wal.Since(stream.Context(), req.GetVersionIndex())
	if err != nil {
		return err
	}

	sub := &subscriber{
		key:     subscriberKey(req.GetPodIp(), req.GetAgentId()),
		podIP:   req.GetPodIp(),
		agentID: req.GetAgentId(),
		ch:      make(chan *ChangeEnvelope, len(backlog)+subscriberBuffer),
	}
	for _, change := range backlog {
		msg := cloneEnvelope(change)
		msg.Tier = MessageTier_MESSAGE_TIER_RECOVER
		sub.ch <- msg
	}

	c.mu.Lock()
	c.subscribers[sub.key] = sub
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.subscribers, sub.key)
		close(sub.ch)
		c.mu.Unlock()
	}()

	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case change, ok := <-sub.ch:
			if !ok {
				return nil
			}
			if change == nil {
				continue
			}
			if err := stream.Send(change); err != nil {
				return err
			}
		}
	}
}

func (c *Coordinator) Snapshot() []SubscriberSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]SubscriberSnapshot, 0, len(c.subscribers))
	for _, sub := range c.subscribers {
		out = append(out, SubscriberSnapshot{PodIP: sub.podIP, AgentID: sub.agentID})
	}
	return out
}

func (c *Coordinator) broadcast(change *ChangeEnvelope) {
	if change == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for key, sub := range c.subscribers {
		msg := cloneEnvelope(change)
		select {
		case sub.ch <- msg:
		default:
			close(sub.ch)
			delete(c.subscribers, key)
		}
	}
}

func subscriberKey(podIP, agentID string) string {
	return podIP + "/" + agentID
}

func cloneEnvelope(in *ChangeEnvelope) *ChangeEnvelope {
	if in == nil {
		return nil
	}
	out := *in
	out.Bin = append([]byte(nil), in.Bin...)
	return &out
}

type FileWAL struct {
	dir   string
	store ChangeStore
	mu    sync.Mutex
}

type walSegmentState struct {
	msg   *capnp.Message
	log   EventLog
	dirty bool
}

func NewFileWAL(path string, store ChangeStore) (*FileWAL, error) {
	if store == nil {
		return nil, fmt.Errorf("change store is required")
	}
	w := &FileWAL{dir: path, store: store}
	if err := w.ensureDir(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *FileWAL) Append(ctx context.Context, env *ChangeEnvelope) (*ChangeEnvelope, error) {
	if env == nil {
		return nil, fmt.Errorf("change envelope is required")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	latestIndex, err := w.latestLocked()
	if err != nil {
		return nil, err
	}
	nextIndex := latestIndex + 1
	targetSegment := segmentNumberForIndex(nextIndex)

	latestOverlapIndex := uint64(0)
	segmentNos, err := w.segmentNumbersLocked()
	if err != nil {
		return nil, err
	}
	states := make(map[uint64]*walSegmentState, len(segmentNos))
	for _, segNo := range segmentNos {
		msg, logRoot, err := w.readSegmentLocked(segNo)
		if err != nil {
			return nil, err
		}
		items, err := logRoot.Items()
		if err != nil {
			return nil, err
		}
		state := &walSegmentState{msg: msg, log: logRoot}
		for i := 0; i < items.Len(); i++ {
			item := items.At(i)
			if item.Status() != EventItem_Status_ontop {
				continue
			}
			eventKey, err := item.EventKey()
			if err != nil {
				return nil, err
			}
			if item.EventType() == toWALEventType(env.GetType()) && eventKey == env.GetHostname() {
				item.SetStatus(EventItem_Status_overlaped)
				latestOverlapIndex = item.Index()
				if state.log.NotOverlap() > 0 {
					state.log.SetNotOverlap(state.log.NotOverlap() - 1)
				}
				state.dirty = true
			}
		}
		states[segNo] = state
	}

	target, ok := states[targetSegment]
	if !ok {
		msg, logRoot, err := w.newSegment()
		if err != nil {
			return nil, err
		}
		target = &walSegmentState{msg: msg, log: logRoot}
		states[targetSegment] = target
	}

	existing, err := target.log.Items()
	if err != nil {
		return nil, err
	}
	oldLen := existing.Len()
	nextList, err := target.log.NewItems(oldLen + 1)
	if err != nil {
		return nil, err
	}
	for i := 0; i < oldLen; i++ {
		if err := nextList.Set(i, existing.At(i)); err != nil {
			return nil, err
		}
	}

	item := nextList.At(oldLen)
	item.SetIndex(nextIndex)
	item.SetEventType(toWALEventType(env.GetType()))
	if err := item.SetEventKey(env.GetHostname()); err != nil {
		return nil, err
	}
	item.SetEventAction(EventItem_EventAction_put)
	item.SetLastAffectIndex(latestOverlapIndex)
	item.SetStatus(EventItem_Status_ontop)

	target.log.SetTotal(uint64(oldLen + 1))
	target.log.SetItems(nextList)
	target.log.SetNotOverlap(target.log.NotOverlap() + 1)
	now := uint64(time.Now().Unix())
	if target.log.StartTime() == 0 {
		target.log.SetStartTime(now)
	}
	target.log.SetCloseTime(now)
	target.dirty = true

	cloned := cloneEnvelope(env)
	cloned.VersionIndex = nextIndex

	if err := w.store.Persist(ctx, cloned); err != nil {
		return nil, err
	}
	for _, segNo := range sortedSegmentKeys(states) {
		state := states[segNo]
		if !state.dirty {
			continue
		}
		if err := w.writeSegmentLocked(segNo, state.msg); err != nil {
			return nil, err
		}
	}
	return cloned, nil
}

func (w *FileWAL) Since(ctx context.Context, version uint64) ([]*ChangeEnvelope, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	segmentNos, err := w.segmentNumbersLocked()
	if err != nil {
		return nil, err
	}
	out := make([]*ChangeEnvelope, 0)
	for _, segNo := range segmentNos {
		_, logRoot, err := w.readSegmentLocked(segNo)
		if err != nil {
			return nil, err
		}
		items, err := logRoot.Items()
		if err != nil {
			return nil, err
		}
		for i := 0; i < items.Len(); i++ {
			item := items.At(i)
			if item.Status() != EventItem_Status_ontop || item.Index() <= version {
				continue
			}
			key, err := item.EventKey()
			if err != nil {
				return nil, err
			}
			eventType := fromWALEventType(item.EventType())
			bin, err := w.store.Load(ctx, eventType, key)
			if err != nil {
				continue
			}
			out = append(out, &ChangeEnvelope{
				Cluster:       "default",
				Type:          eventType,
				Hostname:      key,
				Bin:           bin,
				VersionIndex:  item.Index(),
				TimestampUnix: int64(logRoot.CloseTime()),
				Tier:          MessageTier_MESSAGE_TIER_RECOVER,
			})
		}
	}
	return out, nil
}

func (w *FileWAL) Latest(_ context.Context) (uint64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	latest, err := w.latestLocked()
	if err != nil {
		return 0, err
	}
	return latest, nil
}

func (w *FileWAL) ensureDir() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return os.MkdirAll(w.dir, 0o755)
}

func (w *FileWAL) readSegmentLocked(segment uint64) (*capnp.Message, EventLog, error) {
	data, err := os.ReadFile(w.segmentPath(segment))
	if err != nil {
		return nil, EventLog{}, err
	}
	msg, err := capnp.Unmarshal(data)
	if err != nil {
		return nil, EventLog{}, err
	}
	root, err := ReadRootEventLog(msg)
	return msg, root, err
}

func (w *FileWAL) writeSegmentLocked(segment uint64, msg *capnp.Message) error {
	return w.writeSegmentFile(segment, msg)
}

func (w *FileWAL) writeSegmentFile(segment uint64, msg *capnp.Message) error {
	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		return err
	}
	data, err := msg.Marshal()
	if err != nil {
		return err
	}
	path := w.segmentPath(segment)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (w *FileWAL) latestLocked() (uint64, error) {
	segments, err := w.segmentNumbersLocked()
	if err != nil {
		return 0, err
	}
	if len(segments) == 0 {
		return 0, nil
	}
	last := segments[len(segments)-1]
	_, logRoot, err := w.readSegmentLocked(last)
	if err != nil {
		return 0, err
	}
	items, err := logRoot.Items()
	if err != nil {
		return 0, err
	}
	if items.Len() == 0 {
		return 0, nil
	}
	return items.At(items.Len() - 1).Index(), nil
}

func (w *FileWAL) newSegment() (*capnp.Message, EventLog, error) {
	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return nil, EventLog{}, err
	}
	root, err := NewRootEventLog(seg)
	if err != nil {
		return nil, EventLog{}, err
	}
	list, err := root.NewItems(0)
	if err != nil {
		return nil, EventLog{}, err
	}
	if err := root.SetItems(list); err != nil {
		return nil, EventLog{}, err
	}
	now := uint64(time.Now().Unix())
	root.SetStartTime(now)
	root.SetCloseTime(now)
	return msg, root, nil
}

func (w *FileWAL) segmentNumbersLocked() ([]uint64, error) {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]uint64, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "wal.bin.") {
			continue
		}
		raw := strings.TrimPrefix(name, "wal.bin.")
		n, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func (w *FileWAL) segmentPath(segment uint64) string {
	return filepath.Join(w.dir, fmt.Sprintf("wal.bin.%d", segment))
}

func segmentNumberForIndex(index uint64) uint64 {
	if index == 0 {
		return 1
	}
	return ((index - 1) / walItemsPerFile) + 1
}

func sortedSegmentKeys(m map[uint64]*walSegmentState) []uint64 {
	out := make([]uint64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func toWALEventType(t EventType) EventItem_EventType {
	switch t {
	case EventType_EVENT_TYPE_DNS:
		return EventItem_EventType_dns
	case EventType_EVENT_TYPE_TLS:
		return EventItem_EventType_tls
	case EventType_EVENT_TYPE_HTTP:
		return EventItem_EventType_http
	default:
		return EventItem_EventType_dns
	}
}

func fromWALEventType(t EventItem_EventType) EventType {
	switch t {
	case EventItem_EventType_dns:
		return EventType_EVENT_TYPE_DNS
	case EventItem_EventType_tls:
		return EventType_EVENT_TYPE_TLS
	case EventItem_EventType_http:
		return EventType_EVENT_TYPE_HTTP
	default:
		return EventType_EVENT_TYPE_UNSPECIFIED
	}
}
