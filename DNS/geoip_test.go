package dns

import (
	"fmt"
	"math"
	"net"
	"strings"
	"testing"
)

type geoLookupStub struct {
	coords map[string]*Coordinates
}

func (s *geoLookupStub) lookup(ip net.IP) (*Coordinates, error) {
	if ip == nil {
		return nil, nil
	}
	c, ok := s.coords[ip.String()]
	if !ok {
		return nil, fmt.Errorf("IP not found: %s", ip)
	}
	return c, nil
}

func (s *geoLookupStub) Close() error { return nil }

func TestHaversineKnownDistance(t *testing.T) {
	a := Coordinates{Latitude: 40.7128, Longitude: -74.0060}
	b := Coordinates{Latitude: 51.5074, Longitude: -0.1278}
	if got := Haversine(a, b); math.Abs(got-5570) > 20 {
		t.Fatalf("unexpected distance: %.2f", got)
	}
}

func TestSortValuesByDistance(t *testing.T) {
	lookup := &geoLookupStub{
		coords: map[string]*Coordinates{
			"10.0.0.1": {Latitude: 43.6532, Longitude: -79.3832},
			"10.0.0.2": {Latitude: 51.5074, Longitude: -0.1278},
			"10.0.0.3": {Latitude: 35.6762, Longitude: 139.6503},
		},
	}
	client := Coordinates{Latitude: 40.7128, Longitude: -74.0060}
	got := sortValuesByDistance([]string{"10.0.0.3", "10.0.0.2", "10.0.0.1"}, client, lookup)
	if strings.Join(got, ",") != "10.0.0.1,10.0.0.2,10.0.0.3" {
		t.Fatalf("unexpected sorted order: %q", strings.Join(got, ","))
	}
}
