package dns

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"sort"
	"strings"
	"sync"

	capnp "capnproto.org/go/capnp/v3"
	mdns "github.com/miekg/dns"
)

type Engine struct {
	store     ZoneStore
	geoDriver geoIPDriver
	mu        sync.Mutex
	server    *mdns.Server
}

type DNSEngineOptions struct {
	Store    ZoneStore
	MMDBPath string
}

type readerHandler struct {
	zone    Zone
	records DnsRecord_List
	indexes map[uint16][]uint32
}

type lookup interface {
	Lookup(name string, qtype uint16) ([]mdns.RR, error)
}

type storeHandler struct {
	store     ZoneStore
	geoDriver geoIPDriver
}

func NewDNSEngine(opts DNSEngineOptions) *Engine {
	engine := &Engine{store: opts.Store}
	engine.initGeoIP(opts.MMDBPath)
	return engine
}

func (e *Engine) Listen(addr string, ctx context.Context) error {
	server := &mdns.Server{
		Addr:    addr,
		Net:     "udp",
		Handler: &storeHandler{store: e.store, geoDriver: e.geoDriver},
	}

	e.mu.Lock()
	if e.server != nil {
		e.mu.Unlock()
		return fmt.Errorf("dns engine already listening")
	}
	e.server = server
	e.mu.Unlock()

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = e.Stop()
		case <-done:
		}
	}()
	err := server.ListenAndServe()
	close(done)

	e.mu.Lock()
	if e.server == server {
		e.server = nil
	}
	e.mu.Unlock()
	return err
}

func (e *Engine) Stop() error {
	e.mu.Lock()
	server := e.server
	driver := e.geoDriver
	e.server = nil
	e.geoDriver = nil
	e.mu.Unlock()
	if server == nil {
		if driver != nil {
			return driver.Close()
		}
		return nil
	}
	err := server.Shutdown()
	if driver != nil {
		closeErr := driver.Close()
		if err == nil {
			err = closeErr
		}
	}
	return err
}

func newReaderLookup(r io.Reader) (lookup, error) {
	zone, err := Read(r)
	if err != nil {
		return nil, err
	}

	records, err := zone.Records()
	if err != nil {
		return nil, fmt.Errorf("read zone records: %w", err)
	}

	h := &readerHandler{
		zone:    zone,
		records: records,
		indexes: make(map[uint16][]uint32, 8),
	}

	if err := h.loadIndexes(); err != nil {
		return nil, err
	}

	return h, nil
}

func (h *storeHandler) ServeDNS(w mdns.ResponseWriter, req *mdns.Msg) {
	respond(w, req, func(name string, qtype uint16) ([]mdns.RR, error) {
		readerHandler, err := h.lookupForQuestion(name)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, nil
			}
			return nil, err
		}
		return readerHandler.Lookup(name, qtype)
	}, h.geoDriver)
}

func respond(w mdns.ResponseWriter, req *mdns.Msg, lookup func(string, uint16) ([]mdns.RR, error), geo geoIPDriver) {
	resp := new(mdns.Msg)
	resp.SetReply(req)
	resp.Authoritative = true

	for _, q := range req.Question {
		answers, err := lookup(q.Name, q.Qtype)
		if err != nil {
			resp.Rcode = mdns.RcodeServerFailure
			_ = w.WriteMsg(resp)
			return
		}
		sortRRsByClientDistance(geo, w.RemoteAddr(), answers)
		resp.Answer = append(resp.Answer, answers...)
	}

	if len(resp.Answer) == 0 && len(req.Question) > 0 {
		resp.Rcode = mdns.RcodeNameError
	}

	_ = w.WriteMsg(resp)
}

func sortRRsByClientDistance(driver geoIPDriver, addr net.Addr, answers []mdns.RR) {
	if driver == nil || len(answers) <= 1 || addr == nil {
		return
	}
	clientIP := remoteAddrIP(addr)
	if clientIP == nil {
		return
	}
	clientCoords, err := driver.lookup(clientIP)
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

func rrDistance(driver geoIPDriver, client Coordinates, rr mdns.RR) float64 {
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
	coords, err := driver.lookup(ip)
	if err != nil || coords == nil {
		return math.MaxFloat64
	}
	return Haversine(client, *coords)
}

func (h *storeHandler) lookupForQuestion(name string) (lookup, error) {
	for _, zone := range CandidateZones(name) {
		f, err := h.store.OpenZone(zone)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}

		handler, newErr := newReaderLookup(f)
		closeErr := f.Close()
		if newErr != nil {
			return nil, newErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		return handler, nil
	}

	return nil, os.ErrNotExist
}

func (h *readerHandler) Lookup(name string, qtype uint16) ([]mdns.RR, error) {
	offsets := h.indexes[qtype]
	if len(offsets) == 0 {
		return nil, nil
	}

	want := normalizeName(name)
	answers := make([]mdns.RR, 0, len(offsets))
	for _, offset := range offsets {
		if int(offset) >= h.records.Len() {
			return nil, fmt.Errorf("record offset %d out of range", offset)
		}

		record := h.records.At(int(offset))
		recordName, err := record.Name()
		if err != nil {
			return nil, fmt.Errorf("read record name at %d: %w", offset, err)
		}
		if !recordMatchesName(normalizeName(recordName), want) {
			continue
		}

		rr, err := recordToRR(record)
		if err != nil {
			return nil, fmt.Errorf("convert record at %d: %w", offset, err)
		}
		if rr != nil {
			answers = append(answers, rr)
		}
	}

	return answers, nil
}

func recordMatchesName(recordName, queryName string) bool {
	if recordName == queryName {
		return true
	}
	if !strings.HasPrefix(recordName, "*.") {
		return false
	}

	recordLabels := strings.Split(strings.TrimSuffix(recordName, "."), ".")
	queryLabels := strings.Split(strings.TrimSuffix(queryName, "."), ".")
	if len(recordLabels) != len(queryLabels) || len(recordLabels) < 2 {
		return false
	}
	for i := 1; i < len(recordLabels); i++ {
		if recordLabels[i] != queryLabels[i] {
			return false
		}
	}
	return true
}

func (h *readerHandler) loadIndexes() error {
	type indexSpec struct {
		qtype  uint16
		name   string
		getter func() (capnp.UInt32List, error)
	}

	load := func(spec indexSpec) error {
		list, err := spec.getter()
		if err != nil {
			return fmt.Errorf("read %s indexes: %w", spec.name, err)
		}
		offsets := make([]uint32, list.Len())
		for i := range offsets {
			offsets[i] = list.At(i)
		}
		h.indexes[spec.qtype] = offsets
		return nil
	}

	for _, spec := range []indexSpec{
		{mdns.TypeA, "A", h.zone.AIndexes},
		{mdns.TypeAAAA, "AAAA", h.zone.AaaaIndexes},
		{mdns.TypeCNAME, "CNAME", h.zone.CnameIndexes},
		{mdns.TypeMX, "MX", h.zone.MxIndexes},
		{mdns.TypeNS, "NS", h.zone.NsIndexes},
		{mdns.TypePTR, "PTR", h.zone.PtrIndexes},
		{mdns.TypeSOA, "SOA", h.zone.SoaIndexes},
		{mdns.TypeTXT, "TXT", h.zone.TxtIndexes},
	} {
		if err := load(spec); err != nil {
			return err
		}
	}
	return nil
}

func recordToRR(record DnsRecord) (mdns.RR, error) {
	name, err := record.Name()
	if err != nil {
		return nil, err
	}

	header := mdns.RR_Header{
		Name:   normalizeName(name),
		Rrtype: dnsRecordTypeToQType(record.Type()),
		Class:  mdns.ClassINET,
		Ttl:    record.Ttl(),
	}

	switch record.Type() {
	case RecordType_a:
		data, err := record.A()
		if err != nil {
			return nil, err
		}
		ip := make(net.IP, net.IPv4len)
		binary.BigEndian.PutUint32(ip, data.Address())
		return &mdns.A{Hdr: header, A: ip}, nil
	case RecordType_aaaa:
		data, err := record.Aaaa()
		if err != nil {
			return nil, err
		}
		ip := make(net.IP, net.IPv6len)
		binary.BigEndian.PutUint64(ip[0:8], data.AddressHigh())
		binary.BigEndian.PutUint64(ip[8:16], data.AddressLow())
		return &mdns.AAAA{Hdr: header, AAAA: ip}, nil
	case RecordType_cname:
		data, err := record.Cname()
		if err != nil {
			return nil, err
		}
		host, err := data.Host()
		if err != nil {
			return nil, err
		}
		return &mdns.CNAME{Hdr: header, Target: normalizeName(host)}, nil
	case RecordType_mx:
		data, err := record.Mx()
		if err != nil {
			return nil, err
		}
		host, err := data.Exchange()
		if err != nil {
			return nil, err
		}
		return &mdns.MX{Hdr: header, Preference: data.Preference(), Mx: normalizeName(host)}, nil
	case RecordType_ns:
		data, err := record.Ns()
		if err != nil {
			return nil, err
		}
		host, err := data.Host()
		if err != nil {
			return nil, err
		}
		return &mdns.NS{Hdr: header, Ns: normalizeName(host)}, nil
	case RecordType_ptr:
		data, err := record.Ptr()
		if err != nil {
			return nil, err
		}
		host, err := data.Host()
		if err != nil {
			return nil, err
		}
		return &mdns.PTR{Hdr: header, Ptr: normalizeName(host)}, nil
	case RecordType_soa:
		data, err := record.Soa()
		if err != nil {
			return nil, err
		}
		mname, err := data.Mname()
		if err != nil {
			return nil, err
		}
		rname, err := data.Rname()
		if err != nil {
			return nil, err
		}
		return &mdns.SOA{
			Hdr:     header,
			Ns:      normalizeName(mname),
			Mbox:    normalizeName(rname),
			Serial:  data.Serial(),
			Refresh: data.Refresh(),
			Retry:   data.Retry(),
			Expire:  data.Expire(),
			Minttl:  data.Minimum(),
		}, nil
	case RecordType_txt:
		data, err := record.Txt()
		if err != nil {
			return nil, err
		}
		values, err := data.Values()
		if err != nil {
			return nil, err
		}
		txt := make([]string, 0, values.Len())
		for i := 0; i < values.Len(); i++ {
			v, err := values.At(i)
			if err != nil {
				return nil, err
			}
			txt = append(txt, v)
		}
		return &mdns.TXT{Hdr: header, Txt: txt}, nil
	default:
		return nil, fmt.Errorf("unsupported record type %d", record.Type())
	}
}

func dnsRecordTypeToQType(t RecordType) uint16 {
	switch t {
	case RecordType_a:
		return mdns.TypeA
	case RecordType_aaaa:
		return mdns.TypeAAAA
	case RecordType_cname:
		return mdns.TypeCNAME
	case RecordType_mx:
		return mdns.TypeMX
	case RecordType_ns:
		return mdns.TypeNS
	case RecordType_ptr:
		return mdns.TypePTR
	case RecordType_soa:
		return mdns.TypeSOA
	case RecordType_txt:
		return mdns.TypeTXT
	default:
		return 0
	}
}

func normalizeName(name string) string {
	return mdns.Fqdn(strings.ToLower(strings.TrimSpace(name)))
}
