#!/usr/bin/env python3
from __future__ import annotations

import json
import sys
import time
from typing import Any

import dns.message
import dns.query
import dns.rdatatype
import dns.resolver
import requests


MASTER_BASE = "http://127.0.0.1:9000"
DNS_SERVER = "127.0.0.1"
DNS_PORT = 8053


DNS_CASES: list[dict[str, Any]] = [
    {
        "hostname": "app.com",
        "payload": {
            "records": [
                {
                    "type": "a",
                    "name": "app.com.",
                    "ttl": 60,
                    "address": "1.2.3.4",
                },
                {
                    "type": "txt",
                    "name": "app.com.",
                    "ttl": 60,
                    "values": ["site=app"],
                },
            ]
        },
        "queries": [
            {"name": "app.com.", "rtype": "A", "expect": "1.2.3.4"},
            {"name": "app.com.", "rtype": "TXT", "expect": '"site=app"'},
        ],
    },
    {
        "hostname": "api.com",
        "payload": {
            "records": [
                {
                    "type": "a",
                    "name": "api.com.",
                    "ttl": 60,
                    "address": "5.6.7.8",
                }
            ]
        },
        "queries": [
            {"name": "api.com.", "rtype": "A", "expect": "5.6.7.8"},
        ],
    },
    {
        "hostname": "wild.com",
        "payload": {
            "records": [
                {
                    "type": "a",
                    "name": "*.wild.com.",
                    "ttl": 60,
                    "address": "9.9.9.9",
                }
            ]
        },
        "queries": [
            {"name": "foo.wild.com.", "rtype": "A", "expect": "9.9.9.9"},
        ],
    },
]


def publish_dns(hostname: str, payload: dict[str, Any]) -> None:
    response = requests.put(
        f"{MASTER_BASE}/api/master/dns",
        params={"hostname": hostname},
        headers={"Content-Type": "application/json"},
        data=json.dumps(payload),
        timeout=5,
    )
    response.raise_for_status()

def dns_query(name: str, rtype: str) -> list[str]:
    query = dns.message.make_query(name, rtype)
    response = dns.query.udp(query, DNS_SERVER, port=DNS_PORT, timeout=1)

    records: list[str] = []
    want_type = dns.rdatatype.from_text(rtype)

    for rrset in response.answer:
        if rrset.rdtype != want_type:
            continue
        for item in rrset:
            if rtype.upper() == "TXT":
                text = "".join(
                    part.decode("utf-8") if isinstance(part, bytes) else str(part)
                    for part in item.strings
                )
                records.append(f'"{text}"')
            else:
                records.append(item.to_text())
    return records


def assert_eventually(name: str, rtype: str, expected: str, timeout: float = 5.0) -> None:
    deadline = time.time() + timeout
    last: list[str] = []
    while time.time() < deadline:
        try:
            last = dns_query(name, rtype)
        except Exception:
            last = []
        if expected in last:
            return
        time.sleep(0.2)
    raise AssertionError(
        f"DNS query failed name={name} type={rtype} expected={expected!r} got={last!r}"
    )


def main() -> int:
    for case in DNS_CASES:
        publish_dns(case["hostname"], case["payload"])
        for query in case["queries"]:
            assert_eventually(query["name"], query["rtype"], query["expect"])
            print(
                f"ok hostname={case['hostname']} query={query['name']} type={query['rtype']} expect={query['expect']}"
            )
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:  # noqa: BLE001
        print(f"FAIL: {exc}", file=sys.stderr)
        raise
