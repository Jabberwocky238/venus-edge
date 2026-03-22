package dns

import (
	"math"
	"net"
	"os"
	"sort"
	"strings"

	mdns "github.com/miekg/dns"
	geoip2 "github.com/oschwald/geoip2-golang"
)

const earthRadiusKm = 6371.0

type Coordinates struct {
	Latitude  float64
	Longitude float64
}

type ipLookup interface {
	LookupIP(ip net.IP) (*Coordinates, error)
	Close() error
}

type GeoIPReader struct {
	db *geoip2.Reader
}

func NewReader(mmdbPath string) (*GeoIPReader, error) {
	db, err := geoip2.Open(mmdbPath)
	if err != nil {
		return nil, err
	}
	return &GeoIPReader{db: db}, nil
}

func (r *GeoIPReader) Close() error {
	if r.db == nil {
		return nil
	}
	return r.db.Close()
}

func (r *GeoIPReader) LookupIP(ip net.IP) (*Coordinates, error) {
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

func degreesToRadians(deg float64) float64 {
	return deg * math.Pi / 180.0
}

func rrDistance(driver ipLookup, client Coordinates, rr mdns.RR) float64 {
	var ip net.IP
	switch value := rr.(type) {
	case *mdns.A:
		ip = value.A
	case *mdns.AAAA:
		ip = value.AAAA
	default:
		return math.MaxFloat64
	}
	if ip == nil {
		return math.MaxFloat64
	}
	coords, err := driver.LookupIP(ip)
	if err != nil || coords == nil {
		return math.MaxFloat64
	}
	return Haversine(client, *coords)
}

func sortRRsByClientDistance(driver ipLookup, addr net.Addr, answers []mdns.RR) {
	if driver == nil || len(answers) <= 1 || addr == nil {
		return
	}
	clientIP := remoteAddrIP(addr)
	if clientIP == nil {
		return
	}
	clientCoords, err := driver.LookupIP(clientIP)
	if err != nil || clientCoords == nil {
		return
	}

	type rrDist struct {
		rr   mdns.RR
		dist float64
	}
	items := make([]rrDist, 0, len(answers))
	for _, rr := range answers {
		items = append(items, rrDist{rr: rr, dist: rrDistance(driver, *clientCoords, rr)})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].dist < items[j].dist
	})
	for i := range items {
		answers[i] = items[i].rr
	}
}
