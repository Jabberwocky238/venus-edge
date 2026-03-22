package master

import (
	"context"
	"sync"
)

type HTTPPublishHook func(context.Context, *Master, string, HTTPChangeJSON) error

var (
	httpPublishHooksMu sync.RWMutex
	httpPublishHooks   []HTTPPublishHook
)

func RegisterHTTPPublishHook(hook HTTPPublishHook) {
	if hook == nil {
		return
	}
	httpPublishHooksMu.Lock()
	httpPublishHooks = append(httpPublishHooks, hook)
	httpPublishHooksMu.Unlock()
}

func runHTTPPublishHooks(ctx context.Context, m *Master, hostname string, change HTTPChangeJSON) error {
	httpPublishHooksMu.RLock()
	hooks := append([]HTTPPublishHook(nil), httpPublishHooks...)
	httpPublishHooksMu.RUnlock()

	for _, hook := range hooks {
		if err := hook(ctx, m, hostname, change); err != nil {
			return err
		}
	}
	return nil
}
