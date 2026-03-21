package replication

import (
	"fmt"

	"google.golang.org/grpc"
)

type SubscribeHandler interface {
	HandleSubscribe(req *PushChangeRequest, stream ReplicationService_SubscribeServer) error
}

type SubscribeHandlerFunc func(req *PushChangeRequest, stream ReplicationService_SubscribeServer) error

func (f SubscribeHandlerFunc) HandleSubscribe(req *PushChangeRequest, stream ReplicationService_SubscribeServer) error {
	return f(req, stream)
}

type Server struct {
	UnimplementedReplicationServiceServer
	handler SubscribeHandler
}

func NewServer(handler SubscribeHandler) *Server {
	return &Server{handler: handler}
}

func (s *Server) Register(registrar grpc.ServiceRegistrar) {
	RegisterReplicationServiceServer(registrar, s)
}

func (s *Server) Subscribe(req *PushChangeRequest, stream ReplicationService_SubscribeServer) error {
	if s.handler == nil {
		return fmt.Errorf("replication handler is not configured")
	}
	if req == nil {
		return fmt.Errorf("subscribe request is required")
	}
	if req.PodIp == "" {
		return fmt.Errorf("pod_ip is required")
	}
	if req.AgentId == "" {
		return fmt.Errorf("agent_id is required")
	}
	return s.handler.HandleSubscribe(req, stream)
}
