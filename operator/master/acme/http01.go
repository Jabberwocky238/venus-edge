package acme

import (
	"context"
	"fmt"

	master "aaa/operator/master"
)

type HTTP01Solver struct {
	master *master.Master
}

func (s *HTTP01Solver) Present(ctx context.Context, hostname, token, backend string) error {
	if err := ensureMaster(s.master); err != nil {
		return err
	}
	if hostname == "" || token == "" || backend == "" {
		return fmt.Errorf("hostname, token and backend are required")
	}
	return run(ctx, func(ctx context.Context) error {
		bin, err := renderHTTPBin(newHTTPRoute(hostname, token, backend))
		if err != nil {
			return err
		}
		_, err = s.master.PublishHTTP(ctx, hostname, bin)
		return err
	})
}
