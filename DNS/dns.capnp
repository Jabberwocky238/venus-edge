@0xd8c6e9d1a4b2f307;

using Go = import "/go.capnp";
$Go.package("dns");
$Go.import("aaa/DNS");

enum RecordType {
  a @0;
  aaaa @1;
  cname @2;
  mx @3;
  ns @4;
  ptr @5;
  soa @6;
  txt @7;
}

struct ARecord {
  address @0 :UInt32;
}

struct AAAARecord {
  addressHigh @0 :UInt64;
  addressLow @1 :UInt64;
}

struct CNAMERecord {
  host @0 :Text;
}

struct MXRecord {
  preference @0 :UInt16;
  exchange @1 :Text;
}

struct NSRecord {
  host @0 :Text;
}

struct PTRRecord {
  host @0 :Text;
}

struct SOARecord {
  mname @0 :Text;
  rname @1 :Text;
  serial @2 :UInt32;
  refresh @3 :UInt32;
  retry @4 :UInt32;
  expire @5 :UInt32;
  minimum @6 :UInt32;
}

struct TXTRecord {
  values @0 :List(Text);
}

struct DnsRecord {
  name @0 :Text;
  ttl @1 :UInt32;
  type @2 :RecordType;

  union {
    a @3 :ARecord;
    aaaa @4 :AAAARecord;
    cname @5 :CNAMERecord;
    mx @6 :MXRecord;
    ns @7 :NSRecord;
    ptr @8 :PTRRecord;
    soa @9 :SOARecord;
    txt @10 :TXTRecord;
  }
}

struct Zone {
  records @0 :List(DnsRecord);

  aIndexes @1 :List(UInt32);
  aaaaIndexes @2 :List(UInt32);
  cnameIndexes @3 :List(UInt32);
  mxIndexes @4 :List(UInt32);
  nsIndexes @5 :List(UInt32);
  ptrIndexes @6 :List(UInt32);
  soaIndexes @7 :List(UInt32);
  txtIndexes @8 :List(UInt32);
}
