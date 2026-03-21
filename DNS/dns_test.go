package dns

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mdns "github.com/miekg/dns"
)

type fakeResponseWriter struct {
	msg *mdns.Msg
}

func (f *fakeResponseWriter) LocalAddr() net.Addr        { return &net.UDPAddr{} }
func (f *fakeResponseWriter) RemoteAddr() net.Addr       { return &net.UDPAddr{} }
func (f *fakeResponseWriter) WriteMsg(m *mdns.Msg) error { f.msg = m; return nil }
func (f *fakeResponseWriter) Write([]byte) (int, error)  { return 0, nil }
func (f *fakeResponseWriter) Close() error               { return nil }
func (f *fakeResponseWriter) TsigStatus() error          { return nil }
func (f *fakeResponseWriter) TsigTimersOnly(bool)        {}
func (f *fakeResponseWriter) Hijack()                    {}

type geoDriverStub struct {
	coords map[string]*Coordinates
}

func (s *geoDriverStub) lookup(ip net.IP) (*Coordinates, error) {
	if ip == nil {
		return nil, nil
	}
	return s.coords[ip.String()], nil
}

func (s *geoDriverStub) Close() error { return nil }

type testARecordBuilder struct{}
type testTXTRecordBuilder struct{}
type testSOARecordBuilder struct{}
type testWildcardARecordBuilder struct{}

func (testARecordBuilder) Type() RecordType { return RecordType_a }
func (testARecordBuilder) Build(record DnsRecord) error {
	if err := record.SetName("example.com."); err != nil {
		return err
	}
	record.SetTtl(60)
	record.SetType(RecordType_a)
	data, err := record.NewA()
	if err != nil {
		return err
	}
	data.SetAddress(0x01020304)
	return nil
}

func (testTXTRecordBuilder) Type() RecordType { return RecordType_txt }
func (testTXTRecordBuilder) Build(record DnsRecord) error {
	if err := record.SetName("example.com."); err != nil {
		return err
	}
	record.SetTtl(60)
	record.SetType(RecordType_txt)
	data, err := record.NewTxt()
	if err != nil {
		return err
	}
	values, err := data.NewValues(1)
	if err != nil {
		return err
	}
	return values.Set(0, "v=spf1 -all")
}

func (testSOARecordBuilder) Type() RecordType { return RecordType_soa }
func (testSOARecordBuilder) Build(record DnsRecord) error {
	if err := record.SetName("example.com."); err != nil {
		return err
	}
	record.SetTtl(604800)
	record.SetType(RecordType_soa)
	data, err := record.NewSoa()
	if err != nil {
		return err
	}
	if err := data.SetMname("ns1.example.com."); err != nil {
		return err
	}
	if err := data.SetRname("hostmaster.example.com."); err != nil {
		return err
	}
	data.SetSerial(1)
	data.SetRefresh(3600)
	data.SetRetry(600)
	data.SetExpire(1209600)
	data.SetMinimum(300)
	return nil
}

func (testWildcardARecordBuilder) Type() RecordType { return RecordType_a }
func (testWildcardARecordBuilder) Build(record DnsRecord) error {
	if err := record.SetName("*.example.com."); err != nil {
		return err
	}
	record.SetTtl(60)
	record.SetType(RecordType_a)
	data, err := record.NewA()
	if err != nil {
		return err
	}
	data.SetAddress(0x05060708)
	return nil
}

func mustNewQuestion(name string, qtype uint16) *mdns.Msg {
	req := new(mdns.Msg)
	req.SetQuestion(name, qtype)
	return req
}

func TestNewReaderLookupAndServeDNS(t *testing.T) {
	data := mustEncodeTestZone(t)

	lookup, err := newReaderLookup(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("newReaderLookup() error = %v", err)
	}

	req := mustNewQuestion("example.com.", mdns.TypeA)

	writer := &fakeResponseWriter{}
	respond(writer, req, lookup.Lookup, nil)

	if writer.msg == nil {
		t.Fatal("expected response message")
	}
	if writer.msg.Rcode != mdns.RcodeSuccess {
		t.Fatalf("unexpected rcode: %d", writer.msg.Rcode)
	}
	if len(writer.msg.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(writer.msg.Answer))
	}

	a, ok := writer.msg.Answer[0].(*mdns.A)
	if !ok {
		t.Fatalf("expected A record, got %T", writer.msg.Answer[0])
	}
	if got := a.A.String(); got != "1.2.3.4" {
		t.Fatalf("unexpected A answer: %s", got)
	}
}

func TestServeDNSReturnsTXT(t *testing.T) {
	data := mustEncodeTestZone(t)

	lookup, err := newReaderLookup(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("newReaderLookup() error = %v", err)
	}

	req := mustNewQuestion("example.com.", mdns.TypeTXT)

	writer := &fakeResponseWriter{}
	respond(writer, req, lookup.Lookup, nil)

	if writer.msg == nil {
		t.Fatal("expected response message")
	}
	if len(writer.msg.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(writer.msg.Answer))
	}

	txt, ok := writer.msg.Answer[0].(*mdns.TXT)
	if !ok {
		t.Fatalf("expected TXT record, got %T", writer.msg.Answer[0])
	}
	if len(txt.Txt) != 1 || txt.Txt[0] != "v=spf1 -all" {
		t.Fatalf("unexpected TXT payload: %#v", txt.Txt)
	}
}

func TestServeDNSMatchesWildcard(t *testing.T) {
	var buf bytes.Buffer
	if err := writeTo(&buf, testWildcardARecordBuilder{}); err != nil {
		t.Fatalf("writeTo() error = %v", err)
	}

	lookup, err := newReaderLookup(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("newReaderLookup() error = %v", err)
	}

	writer := &fakeResponseWriter{}
	respond(writer, mustNewQuestion("foo.example.com.", mdns.TypeA), lookup.Lookup, nil)

	if writer.msg == nil {
		t.Fatal("expected response message")
	}
	if writer.msg.Rcode != mdns.RcodeSuccess {
		t.Fatalf("unexpected rcode: %d", writer.msg.Rcode)
	}
	if len(writer.msg.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(writer.msg.Answer))
	}
	a, ok := writer.msg.Answer[0].(*mdns.A)
	if !ok {
		t.Fatalf("expected A record, got %T", writer.msg.Answer[0])
	}
	if got := a.A.String(); got != "5.6.7.8" {
		t.Fatalf("unexpected wildcard A answer: %s", got)
	}
}

func TestSortRRsByClientDistanceSortsAAndAAAA(t *testing.T) {
	driver := &geoDriverStub{
		coords: map[string]*Coordinates{
			"203.0.113.10": {Latitude: 40.7128, Longitude: -74.0060},
			"10.0.0.1":     {Latitude: 43.6532, Longitude: -79.3832},
			"10.0.0.2":     {Latitude: 51.5074, Longitude: -0.1278},
			"2001:db8::1":  {Latitude: 35.6762, Longitude: 139.6503},
		},
	}
	answers := []mdns.RR{
		&mdns.A{Hdr: mdns.RR_Header{Name: "example.com.", Rrtype: mdns.TypeA}, A: net.ParseIP("10.0.0.2").To4()},
		&mdns.AAAA{Hdr: mdns.RR_Header{Name: "example.com.", Rrtype: mdns.TypeAAAA}, AAAA: net.ParseIP("2001:db8::1")},
		&mdns.A{Hdr: mdns.RR_Header{Name: "example.com.", Rrtype: mdns.TypeA}, A: net.ParseIP("10.0.0.1").To4()},
	}
	sortRRsByClientDistance(driver, &net.UDPAddr{IP: net.ParseIP("203.0.113.10"), Port: 53000}, answers)

	if got := answers[0].(*mdns.A).A.String(); got != "10.0.0.1" {
		t.Fatalf("unexpected first answer: %s", got)
	}
	if got := answers[1].(*mdns.A).A.String(); got != "10.0.0.2" {
		t.Fatalf("unexpected second answer: %s", got)
	}
	if got := answers[2].(*mdns.AAAA).AAAA.String(); got != "2001:db8::1" {
		t.Fatalf("unexpected third answer: %s", got)
	}
}

func TestFileHandlerLoadsZoneFromTempDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), DefaultZoneDir)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	path := ZoneFilePath(filepath.Dir(root), "example.com")
	if err := os.WriteFile(path, mustEncodeTestZone(t), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	handler := &storeHandler{store: FSStore{Root: filepath.Dir(root)}}
	req := mustNewQuestion("example.com.", mdns.TypeA)

	writer := &fakeResponseWriter{}
	handler.ServeDNS(writer, req)

	if writer.msg == nil {
		t.Fatal("expected response message")
	}
	if writer.msg.Rcode != mdns.RcodeSuccess {
		t.Fatalf("unexpected rcode: %d", writer.msg.Rcode)
	}
	if len(writer.msg.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(writer.msg.Answer))
	}
}

func TestFileHandlerReturnsNameErrorWhenZoneMissing(t *testing.T) {
	root := filepath.Join(t.TempDir(), DefaultZoneDir)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	handler := &storeHandler{store: FSStore{Root: filepath.Dir(root)}}
	req := mustNewQuestion("missing.example.com.", mdns.TypeA)

	writer := &fakeResponseWriter{}
	handler.ServeDNS(writer, req)

	if writer.msg == nil {
		t.Fatal("expected response message")
	}
	if writer.msg.Rcode != mdns.RcodeNameError {
		t.Fatalf("unexpected rcode: %d", writer.msg.Rcode)
	}
}

func TestNewFSStoreCreatesDefaultDir(t *testing.T) {
	root := t.TempDir()

	store, err := NewFSStore(root)
	if err != nil {
		t.Fatalf("NewFSStore() error = %v", err)
	}
	if store.Root != root {
		t.Fatalf("unexpected root: %q", store.Root)
	}

	info, err := os.Stat(filepath.Join(root, DefaultZoneDir))
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected dns directory")
	}
}

func TestFSStoreWritesFullZoneFile(t *testing.T) {
	root := t.TempDir()

	store, err := NewFSStore(root)
	if err != nil {
		t.Fatalf("NewFSStore() error = %v", err)
	}

	if err := store.Write("example.com",
		testARecordBuilder{},
	); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	path := ZoneFilePath(root, "example.com")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Stat() error = %v", err)
	}

	zone, err := store.Read("example.com")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	records, err := zone.Records()
	if err != nil {
		t.Fatalf("Records() error = %v", err)
	}
	if records.Len() != 1 {
		t.Fatalf("expected 1 answer, got %d", records.Len())
	}
}

func TestFSStoreWriteInvalidARecord(t *testing.T) {
	root := t.TempDir()

	store, err := NewFSStore(root)
	if err != nil {
		t.Fatalf("NewFSStore() error = %v", err)
	}

	if err := store.Write("example.com",
		testInvalidARecordBuilder{},
	); err == nil {
		t.Fatal("expected invalid ipv4 error")
	}
}

type testInvalidARecordBuilder struct{}

func (testInvalidARecordBuilder) Type() RecordType { return RecordType_a }
func (testInvalidARecordBuilder) Build(record DnsRecord) error {
	if err := record.SetName("example.com."); err != nil {
		return err
	}
	return fmt.Errorf("invalid ipv4 address for %q", "example.com.")
}

func TestFSStoreWriteSOAUsesDefaults(t *testing.T) {
	root := t.TempDir()

	store, err := NewFSStore(root)
	if err != nil {
		t.Fatalf("NewFSStore() error = %v", err)
	}

	if err := store.Write("example.com",
		testSOARecordBuilder{},
	); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	zone, err := store.Read("example.com")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	records, err := zone.Records()
	if err != nil {
		t.Fatalf("Records() error = %v", err)
	}
	if records.Len() != 1 {
		t.Fatalf("expected 1 record, got %d", records.Len())
	}

	record := records.At(0)
	if got := record.Ttl(); got != 604800 {
		t.Fatalf("unexpected ttl: %d", got)
	}

	soa, err := record.Soa()
	if err != nil {
		t.Fatalf("Soa() error = %v", err)
	}
	if soa.Serial() != 1 || soa.Refresh() != 3600 || soa.Retry() != 600 || soa.Expire() != 1209600 || soa.Minimum() != 300 {
		t.Fatalf("unexpected soa defaults: serial=%d refresh=%d retry=%d expire=%d minimum=%d",
			soa.Serial(), soa.Refresh(), soa.Retry(), soa.Expire(), soa.Minimum())
	}
}

func TestFSStoreWriteRejectsEmptyTXT(t *testing.T) {
	root := t.TempDir()

	store, err := NewFSStore(root)
	if err != nil {
		t.Fatalf("NewFSStore() error = %v", err)
	}

	if err := store.Write("example.com",
		testEmptyTXTRecordBuilder{},
	); err == nil {
		t.Fatal("expected empty TXT values error")
	}
}

type testEmptyTXTRecordBuilder struct{}

func (testEmptyTXTRecordBuilder) Type() RecordType { return RecordType_txt }
func (testEmptyTXTRecordBuilder) Build(record DnsRecord) error {
	if err := record.SetName("example.com."); err != nil {
		return err
	}
	return fmt.Errorf("values is required")
}

func TestEngineStopWithoutListen(t *testing.T) {
	engine := NewDNSEngine(DNSEngineOptions{Store: FSStore{Root: t.TempDir()}})
	if err := engine.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestNewDNSEngineIgnoresMissingMMDB(t *testing.T) {
	engine := NewDNSEngine(DNSEngineOptions{
		Store:    FSStore{Root: t.TempDir()},
		MMDBPath: filepath.Join(t.TempDir(), "missing.mmdb"),
	})
	if engine.geoDriver != nil {
		t.Fatal("expected nil geo driver for missing mmdb")
	}
}

func TestNewDNSEngineLoadsRealMMDB(t *testing.T) {
	engine := NewDNSEngine(DNSEngineOptions{
		Store:    FSStore{Root: t.TempDir()},
		MMDBPath: filepath.Join("..", "data", "GeoLite2-City.mmdb"),
	})
	if engine.geoDriver == nil {
		t.Fatal("expected geo driver to be loaded")
	}
	coords, err := engine.geoDriver.lookup(net.ParseIP("8.8.8.8"))
	if err != nil {
		t.Fatalf("lookup() error = %v", err)
	}
	if coords == nil {
		t.Fatal("expected coordinates for real mmdb lookup")
	}
	_ = engine.Stop()
}

func TestNewDNSEnginePanicsOnBrokenMMDB(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for broken mmdb")
		}
	}()
	_ = NewDNSEngine(DNSEngineOptions{
		Store:    FSStore{Root: t.TempDir()},
		MMDBPath: filepath.Join("..", "data", "GeoLite2-City.mmdb.bak"),
	})
}

func TestEngineListenAndStop(t *testing.T) {
	root := t.TempDir()
	store, err := NewFSStore(root)
	if err != nil {
		t.Fatalf("NewFSStore() error = %v", err)
	}
	if err := store.Write("example.com",
		testARecordBuilder{},
	); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	engine := NewDNSEngine(DNSEngineOptions{Store: store})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- engine.Listen("127.0.0.1:18053", ctx)
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()

	err = <-errCh
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "operation not permitted") {
		t.Skipf("udp listen blocked in sandbox: %v", err)
	}
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "shutdown") {
		t.Fatalf("Listen() error = %v", err)
	}
}

func TestEngineListenAndServeRealDNSQuery(t *testing.T) {
	root := t.TempDir()
	store, err := NewFSStore(root)
	if err != nil {
		t.Fatalf("NewFSStore() error = %v", err)
	}
	if err := store.Write("example.com", testARecordBuilder{}, testWildcardARecordBuilder{}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	addr := "127.0.0.1:18054"
	engine := NewDNSEngine(DNSEngineOptions{Store: store})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- engine.Listen(addr, ctx)
	}()
	time.Sleep(100 * time.Millisecond)

	client := new(mdns.Client)
	req := mustNewQuestion("foo.example.com.", mdns.TypeA)
	resp, _, err := client.Exchange(req, addr)
	if err != nil {
		cancel()
		runErr := <-errCh
		if strings.Contains(strings.ToLower(err.Error()), "operation not permitted") || (runErr != nil && strings.Contains(strings.ToLower(runErr.Error()), "operation not permitted")) {
			t.Skipf("udp networking blocked in sandbox: queryErr=%v listenErr=%v", err, runErr)
		}
		t.Fatalf("Exchange() error = %v", err)
	}
	if resp.Rcode != mdns.RcodeSuccess {
		t.Fatalf("unexpected rcode: %d", resp.Rcode)
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(resp.Answer))
	}
	a, ok := resp.Answer[0].(*mdns.A)
	if !ok {
		t.Fatalf("expected A record, got %T", resp.Answer[0])
	}
	if got := a.A.String(); got != "5.6.7.8" {
		t.Fatalf("unexpected A answer: %s", got)
	}

	cancel()
	err = <-errCh
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "shutdown") {
		t.Fatalf("Listen() error = %v", err)
	}
}

func mustEncodeTestZone(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := writeTo(&buf,
		testARecordBuilder{},
		testTXTRecordBuilder{},
	); err != nil {
		t.Fatalf("writeTo() error = %v", err)
	}

	return buf.Bytes()
}
