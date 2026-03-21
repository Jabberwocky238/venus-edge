package master

import (
	"fmt"
	"sync"

	"aaa/operator/replication"
)

const subscriberBuffer = 16

type subscriber struct {
	podIP   string
	agentID string
	ch      chan *replication.ChangeEnvelope
}

type SubscriberSnapshot struct {
	PodIP   string `json:"pod_ip"`
	AgentID string `json:"agent_id"`
}

type Hub struct {
	mu          sync.RWMutex
	subscribers map[string]*subscriber
}

func NewHub() *Hub {
	return &Hub{
		subscribers: make(map[string]*subscriber),
	}
}

func (h *Hub) Publish(change *replication.ChangeEnvelope) *replication.PushChangeResponse {
	if change == nil {
		return &replication.PushChangeResponse{Accepted: false, Message: "change is required"}
	}

	h.mu.RLock()
	subs := make([]*subscriber, 0, len(h.subscribers))
	for _, sub := range h.subscribers {
		subs = append(subs, sub)
	}
	h.mu.RUnlock()

	delivered := 0
	for _, sub := range subs {
		select {
		case sub.ch <- change:
			delivered++
		default:
		}
	}

	return &replication.PushChangeResponse{
		Accepted: delivered > 0,
		Message:  fmt.Sprintf("delivered to %d subscribers", delivered),
	}
}

func (h *Hub) HandleSubscribe(req *replication.PushChangeRequest, stream replication.ReplicationService_SubscribeServer) error {
	sub := &subscriber{
		podIP:   req.GetPodIp(),
		agentID: req.GetAgentId(),
		ch:      make(chan *replication.ChangeEnvelope, subscriberBuffer),
	}
	key := subscriberKey(sub.podIP, sub.agentID)

	h.mu.Lock()
	h.subscribers[key] = sub
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.subscribers, key)
		h.mu.Unlock()
		close(sub.ch)
	}()

	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case change := <-sub.ch:
			if change == nil {
				continue
			}
			if err := stream.Send(change); err != nil {
				return err
			}
		}
	}
}

func subscriberKey(podIP, agentID string) string {
	return podIP + "/" + agentID
}

func (h *Hub) Snapshot() []SubscriberSnapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()

	snapshots := make([]SubscriberSnapshot, 0, len(h.subscribers))
	for _, sub := range h.subscribers {
		snapshots = append(snapshots, SubscriberSnapshot{
			PodIP:   sub.podIP,
			AgentID: sub.agentID,
		})
	}
	return snapshots
}

var _ replication.SubscribeHandler = (*Hub)(nil)
var _ interface {
	Publish(*replication.ChangeEnvelope) *replication.PushChangeResponse
	HandleSubscribe(*replication.PushChangeRequest, replication.ReplicationService_SubscribeServer) error
} = (*Hub)(nil)
