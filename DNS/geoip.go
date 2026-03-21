package dns

import (
	"math"
	"net"
	"os"
	"sort"
	"strings"

	geoip2 "github.com/oschwald/geoip2-golang"
)

const earthRadiusKm = 6371.0

type Coordinates struct {
	Latitude  float64
	Longitude float64
}

type geoIPLookup interface {
	lookup(ip net.IP) (*Coordinates, error)
}

type geoIPDriver interface {
	geoIPLookup
	Close() error
}

type Reader struct {
	db *geoip2.Reader
}

func NewReader(mmdbPath string) (*Reader, error) {
	db, err := geoip2.Open(mmdbPath)
	if err != nil {
		return nil, err
	}
	return &Reader{db: db}, nil
}

func (r *Reader) Close() error {
	if r.db == nil {
		return nil
	}
	return r.db.Close()
}

func (r *Reader) lookup(ip net.IP) (*Coordinates, error) {
	city, err := r.db.City(ip)
	if err != nil {
		return nil, err
	}
	return &Coordinates{
		Latitude:  city.Location.Latitude,
		Longitude: city.Location.Longitude,
	}, nil
}

func (e *Engine) initGeoIP(mmdbPath string) {
	if strings.TrimSpace(mmdbPath) == "" {
		return
	}
	if _, err := os.Stat(mmdbPath); err != nil {
		if os.IsNotExist(err) {
			return
		}
		panic(err)
	}
	reader, err := NewReader(mmdbPath)
	if err != nil {
		panic(err)
	}
	e.geoDriver = reader
}

func remoteAddrIP(addr net.Addr) net.IP {
	if addr == nil {
		return nil
	}
	switch value := addr.(type) {
	case *net.UDPAddr:
		return value.IP
	case *net.TCPAddr:
		return value.IP
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return net.ParseIP(addr.String())
	}
	return net.ParseIP(host)
}

func Haversine(a, b Coordinates) float64 {
	lat1 := degreesToRadians(a.Latitude)
	lat2 := degreesToRadians(b.Latitude)
	dLat := degreesToRadians(b.Latitude - a.Latitude)
	dLon := degreesToRadians(b.Longitude - a.Longitude)

	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(h), math.Sqrt(1-h))
	return earthRadiusKm * c
}

func sortValuesByDistance(values []string, client Coordinates, lookup geoIPLookup) []string {
	if len(values) <= 1 {
		return values
	}
	type ipDist struct {
		value string
		dist  float64
	}
	entries := make([]ipDist, 0, len(values))
	for _, value := range values {
		ip := net.ParseIP(value)
		if ip == nil {
			entries = append(entries, ipDist{value: value, dist: math.MaxFloat64})
			continue
		}
		coords, err := lookup.lookup(ip)
		if err != nil || coords == nil {
			entries = append(entries, ipDist{value: value, dist: math.MaxFloat64})
			continue
		}
		entries = append(entries, ipDist{value: value, dist: Haversine(client, *coords)})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].dist < entries[j].dist
	})
	sorted := make([]string, 0, len(entries))
	for _, entry := range entries {
		sorted = append(sorted, entry.value)
	}
	return sorted
}

func degreesToRadians(deg float64) float64 {
	return deg * math.Pi / 180.0
}
