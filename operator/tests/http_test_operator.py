#!/usr/bin/env python3
from __future__ import annotations

import json
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any

import requests


MASTER_BASE = "http://127.0.0.1:9000"
PROXY_BASE = "http://127.0.0.1:8080"
BACKEND_PORT = 18080
BACKEND_BASE = f"http://127.0.0.1:{BACKEND_PORT}"
HOSTNAME = "app.com"


class EchoHandler(BaseHTTPRequestHandler):
    server_version = "OperatorHTTPTest/1.0"

    def do_GET(self) -> None:
        body = json.dumps(
            {
                "path": self.path.split("?", 1)[0],
                "query": self.path.split("?", 1)[1] if "?" in self.path else "",
                "headers": {k.lower(): v for k, v in self.headers.items()},
            }
        ).encode("utf-8")

        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, fmt: str, *args: object) -> None:
        return


HTTP_CASES: list[dict[str, Any]] = [
    {
        "name": "prefix",
        "policy": {
            "backend": BACKEND_BASE,
            "pathname_kind": "prefix",
            "pathname": "/prefix",
        },
        "request": {
            "path": "/prefix/demo",
        },
        "expect": {
            "path": "/demo",
        },
    },
    {
        "name": "exact",
        "policy": {
            "backend": BACKEND_BASE,
            "pathname_kind": "exact",
            "pathname": "/exact",
        },
        "request": {
            "path": "/exact",
        },
        "expect": {
            "path": "/exact",
        },
    },
    {
        "name": "query",
        "policy": {
            "backend": BACKEND_BASE,
            "query_items": [
                {"key": "env", "value": "prod"},
            ],
        },
        "request": {
            "path": "/query?env=prod",
        },
        "expect": {
            "path": "/query",
            "query_contains": "env=prod",
        },
    },
    {
        "name": "header",
        "policy": {
            "backend": BACKEND_BASE,
            "header_items": [
                {"key": "x-env", "value": "prod"},
            ],
        },
        "request": {
            "path": "/header",
            "headers": {
                "x-env": "prod",
            },
        },
        "expect": {
            "path": "/header",
            "header": {"x-env": "prod"},
        },
    },
    {
        "name": "regex",
        "policy": {
            "backend": BACKEND_BASE,
            "pathname_kind": "regex",
            "pathname": r"^/items/[0-9]+$",
        },
        "request": {
            "path": "/items/42",
        },
        "expect": {
            "path": "/items/42",
        },
    },
]


def start_backend() -> ThreadingHTTPServer:
    server = ThreadingHTTPServer(("127.0.0.1", BACKEND_PORT), EchoHandler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    return server


def publish_http(payload: dict[str, Any]) -> None:
    response = requests.put(
        f"{MASTER_BASE}/api/master/http",
        params={"hostname": HOSTNAME},
        headers={"Content-Type": "application/json"},
        data=json.dumps(payload),
        timeout=5,
    )
    response.raise_for_status()


def request_proxy(path: str, headers: dict[str, str] | None = None) -> requests.Response:
    return requests.get(
        f"{PROXY_BASE}{path}",
        headers={
            "Host": HOSTNAME,
            **(headers or {}),
        },
        timeout=3,
    )


def assert_eventually(case: dict[str, Any], timeout: float = 5.0) -> None:
    deadline = time.time() + timeout
    last_error = "no response"
    while time.time() < deadline:
        try:
            response = request_proxy(
                case["request"]["path"],
                case["request"].get("headers"),
            )
            if response.status_code != 200:
                last_error = f"status={response.status_code} body={response.text!r}"
                time.sleep(0.2)
                continue

            data = response.json()
            if data.get("path") != case["expect"]["path"]:
                last_error = f"path={data.get('path')!r}"
                time.sleep(0.2)
                continue

            query_contains = case["expect"].get("query_contains")
            if query_contains and query_contains not in data.get("query", ""):
                last_error = f"query={data.get('query')!r}"
                time.sleep(0.2)
                continue

            header_expect = case["expect"].get("header")
            if header_expect:
                for key, value in header_expect.items():
                    if data.get("headers", {}).get(key) != value:
                        last_error = f"headers={data.get('headers')!r}"
                        time.sleep(0.2)
                        break
                else:
                    return
                continue

            return
        except Exception as exc:  # noqa: BLE001
            last_error = str(exc)
            time.sleep(0.2)

    raise AssertionError(f"HTTP case {case['name']} failed: {last_error}")


def main() -> int:
    print("note: suffix matching is not implemented by the current HTTP engine, so it is not asserted here.")
    backend = start_backend()
    try:
        for case in HTTP_CASES:
            payload = {
                "name": HOSTNAME,
                "policies": [case["policy"]],
            }
            publish_http(payload)
            assert_eventually(case)
            print(f"ok case={case['name']}")
    finally:
        backend.shutdown()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
