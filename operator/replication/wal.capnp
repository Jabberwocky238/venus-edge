@0xc4f4d6e1d8b1a271;

using Go = import "/go.capnp";
$Go.package("replication");
$Go.import("replication");

struct EventItem {
  index @0 :UInt64;
  eventType @1 :EventType;
  eventAction @2 :EventAction;
  lastAffectIndex @3 :UInt64;
  status @4 :Status;

  enum EventType {
    dns @0;
    tls @1;
    http @2;
  }

  enum EventAction {
    put @0;
    del @1;
  }

  enum Status {
    done @0;
    pending @1;
  }
}

struct EventLog {
  items @0 :List(EventItem);
  total @1 :UInt64;
  startTime @2 :UInt64;
  closeTime @3 :UInt64;
}

