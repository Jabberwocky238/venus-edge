package ingress

import (
	"context"
	"io"
	"log"
	"net"
	"os"
)

type EngineOptions struct {
	Root       string
	TLSAddr    string
	HTTPAddr   string
	LogEnabled int
}

type Engine struct {
	http   *HTTPEngine
	tls    *TLSEngine
	logger *log.Logger
}

func NewEngine(opts EngineOptions) *Engine {
	root := opts.Root
	if root == "" {
		root = DefaultIngressRoot
	}

	store, err := NewFSStore(root)
	if err != nil {
		panic(err)
	}

	var output io.Writer = io.Discard
	if opts.LogEnabled != 0 {
		output = os.Stderr
	}

	httpEngine := NewHTTPEngine(HTTPEngineOptions{
		Root: root,
		Addr: opts.HTTPAddr,
	})

	return &Engine{
		http: httpEngine,
		tls: NewTLSEngine(TLSEngineOptions{
			Addr:   opts.TLSAddr,
			Finder: NewStoreTLSPolicyFinder(store),
			Handle: httpEngine.ServeConn,
		}),
		logger: log.New(output, "ingress: ", log.LstdFlags),
	}
}

func (e *Engine) Listen(ctx context.Context) error {
	if e.http != nil && e.http.addr != "" {
		go func() {
			if err := e.http.Listen(ctx); err != nil && ctx.Err() == nil {
				e.logf("http listen failed: %v", err)
			}
		}()
	}
	if e.tls == nil || e.tls.addr == "" {
		<-ctx.Done()
		return nil
	}
	return e.tls.Listen(ctx)
}

func (e *Engine) Stop() error {
	if e.http != nil {
		if err := e.http.Stop(); err != nil {
			return err
		}
	}
	if e.tls != nil {
		return e.tls.Stop()
	}
	return nil
}

func (e *Engine) Addr() net.Addr {
	if e.tls == nil {
		return nil
	}
	return e.tls.Addr()
}

func (e *Engine) HTTPAddr() net.Addr {
	if e.http == nil {
		return nil
	}
	return e.http.Addr()
}

func (e *Engine) logf(format string, args ...any) {
	if e.logger != nil {
		e.logger.Printf(format, args...)
	}
}
