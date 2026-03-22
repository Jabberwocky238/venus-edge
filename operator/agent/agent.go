package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	dns "aaa/DNS"
	ingress "aaa/ingress"
	"aaa/operator/replication"

	"github.com/google/uuid"
)

const (
	agentLogPrefix = "\033[38;5;45m[AGENT]\033[0m"
	agentLogOK     = "\033[32m"
	agentLogFail   = "\033[31m"
	agentLogReset  = "\033[0m"
)

type Agent struct {
	root         string
	walManager   *replication.WALManager
	versionStore replication.VersionStore
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
	a := &Agent{root: root}
	wal, err := replication.NewFileWAL(filepath.Join(root, "agent", "wal"), a)
	if err != nil {
		return nil, err
	}
	walManager, err := replication.NewWALManager("default", wal, a)
	if err != nil {
		return nil, err
	}
	a.walManager = walManager
	a.versionStore = walManager
	return a, nil
}

func (a *Agent) Apply(ctx context.Context, change *replication.ChangeEnvelope) error {
	if change == nil {
		return fmt.Errorf("change is required")
	}
	if change.Hostname == "" {
		return fmt.Errorf("hostname is required")
	}
	if len(change.Bin) == 0 {
		return fmt.Errorf("bin is required")
	}
	target, err := a.targetPath(change.Type, change.Hostname)
	if err != nil {
		logAgentApply(change, "", err)
		return err
	}
	if err := a.walManager.PersistChange(ctx, change); err != nil {
		logAgentApply(change, target, err)
		return err
	}
	logAgentApply(change, target, nil)
	return nil
}

func (a *Agent) Persist(ctx context.Context, change *replication.ChangeEnvelope) error {
	if change == nil {
		return fmt.Errorf("change is required")
	}
	target, err := a.targetPath(change.Type, change.Hostname)
	if err != nil {
		return err
	}
	return a.writeAtomically(target, change.Bin)
}

func (a *Agent) Load(_ context.Context, kind replication.EventType, hostname string) ([]byte, error) {
	target, err := a.targetPath(kind, hostname)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(target)
}

func (a *Agent) Subscribe(ctx context.Context, client *replication.Client, podIP, agentID string) error {
	if client == nil {
		return fmt.Errorf("replication client is required")
	}
	follower, err := replication.NewFollower(client, a, a.versionStore, podIP, agentID)
	if err != nil {
		return err
	}
	return follower.Run(ctx)
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
