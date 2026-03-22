@0xc4f4d6e1d8b1a271;

using Go = import "/go.capnp";
$Go.package("schema");
$Go.import("ingress");

struct TlsZone {
  name @0 :Text;
  tlsPolicy @1 :TlsPolicy;
}

struct HttpZone {
  name @0 :Text;
  httpPolicies @1 :List(HttpPolicy);
}

struct HttpPolicy {
  backend @0 :Text;

  union {
    pathname @1 :Pathname;
    query @2 :Query;
    header @3 :Header;
  }

  fixContent @4 :Text;
  allowRawAccess @5 :Bool;
}

struct Pathname {
  kind @0 :Kind;

  union {
    exact @1 :Text;
    prefix @2 :Text;
    regex @3 :Text;
  }

  enum Kind {
    exact @0;
    prefix @1;
    regex @2;
  }
}

struct Query {
  items @0 :List(KeyValue);
}

struct Header {
  items @0 :List(KeyValue);
}

struct KeyValue {
  key @0 :Text;
  value @1 :Text;
}

struct BackendRef {
  hostname @0 :Text;
  port @1 :UInt16;
}

struct TlsPolicy {
  sni @0 :Text;
  certPem @1 :Text;
  keyPem @2 :Text;
  kind @3 :Kind;
  backendRef @4 :BackendRef;

  enum Kind {
    tlsPassthrough @0;
    tlsTerminate @1;
    https @2;
  }
}
