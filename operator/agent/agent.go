package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	dns "aaa/DNS"
	ingress "aaa/ingress"
	"aaa/operator/replication"

	"github.com/google/uuid"
)

type Agent struct {
	root string
}

func New(root string) (*Agent, error) {
	if err := os.MkdirAll(filepath.Join(root, ".venus-edge", "temp"), 0o755); err != nil {
		return nil, err
	}
	if err := dns.EnsureZoneDir(filepath.Join(root, dns.DefaultZoneRoot)); err != nil {
		return nil, err
	}
	if err := ingress.EnsureZoneDirs(filepath.Join(root, ingress.DefaultIngressRoot)); err != nil {
		return nil, err
	}
	return &Agent{root: root}, nil
}

func (a *Agent) HandlePushChange(ctx context.Context, change *replication.ChangeEnvelope) (*replication.PushChangeResponse, error) {
	if change == nil {
		return nil, fmt.Errorf("change is required")
	}
	if change.Hostname == "" {
		return nil, fmt.Errorf("hostname is required")
	}
	if len(change.Bin) == 0 {
		return nil, fmt.Errorf("bin is required")
	}
	target, err := a.targetPath(change.Type, change.Hostname)
	if err != nil {
		return nil, err
	}
	if err := a.writeAtomically(target, change.Bin); err != nil {
		return nil, err
	}
	return &replication.PushChangeResponse{Accepted: true, Message: "applied"}, nil
}

func (a *Agent) Subscribe(ctx context.Context, client *replication.Client, podIP, agentID string) error {
	if client == nil {
		return fmt.Errorf("replication client is required")
	}
	stream, err := client.Subscribe(ctx, podIP, agentID)
	if err != nil {
		return err
	}
	for {
		change, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if _, err := a.HandlePushChange(ctx, change); err != nil {
			return err
		}
	}
}

func (a *Agent) targetPath(kind replication.EventType, hostname string) (string, error) {
	switch kind {
	case replication.EventType_EVENT_TYPE_DNS:
		return dns.ZoneFilePath(filepath.Join(a.root, dns.DefaultZoneRoot), hostname), nil
	case replication.EventType_EVENT_TYPE_TLS:
		return ingress.TLSZoneFilePath(filepath.Join(a.root, ingress.DefaultIngressRoot), hostname), nil
	case replication.EventType_EVENT_TYPE_HTTP:
		return ingress.HTTPZoneFilePath(filepath.Join(a.root, ingress.DefaultIngressRoot), hostname), nil
	default:
		return "", fmt.Errorf("unsupported event type: %v", kind)
	}
}

func (a *Agent) writeAtomically(target string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tempPath := filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+"."+uuid.NewString()+".tmp")
	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, target); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}
