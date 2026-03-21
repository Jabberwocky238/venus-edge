package acme

import (
	"context"
	"fmt"

	master "aaa/operator/master"
)

type DNS01Solver struct {
	master *master.Master
}

func (s *DNS01Solver) Present(ctx context.Context, zone, fqdn, value string) error {
	if err := ensureMaster(s.master); err != nil {
		return err
	}
	if zone == "" || fqdn == "" || value == "" {
		return fmt.Errorf("zone, fqdn and value are required")
	}
	return run(ctx, func(ctx context.Context) error {
		bin, err := renderDNSBin(newTXTRecord(fqdn, value))
		if err != nil {
			return err
		}
		_, err = s.master.PublishDNS(ctx, zone, bin)
		return err
	})
}
