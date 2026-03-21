#!/usr/bin/env python3
from __future__ import annotations

import json
import pathlib
import socket
import ssl
import subprocess
import tempfile
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any

import requests

from multi_host_https_server import ensure_self_signed_cert, serve_https


MASTER_BASE = "http://127.0.0.1:9000"
TLS_FRONTEND = ("127.0.0.1", 8443)
HTTP_BACKEND_PORT = 18081
TERMINATE_BACKEND_PORT = 18082


class EchoHandler(BaseHTTPRequestHandler):
    server_version = "OperatorTLSTest/1.0"

    def do_GET(self) -> None:
        body = json.dumps(
            {
                "tag": self.server.tag,
                "path": self.path,
                "host": self.headers.get("Host", ""),
            }
        ).encode("utf-8")

        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, fmt: str, *args: object) -> None:
        return


def start_http_backend(port: int, tag: str) -> ThreadingHTTPServer:
    server = ThreadingHTTPServer(("127.0.0.1", port), EchoHandler)
    server.tag = tag
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    return server


TLS_CASES: list[dict[str, Any]] = [
    {
        "name": "https",
        "hostname": "https.localhost",
        "tls_payload": {
            "name": "https.localhost",
            "sni": "https.localhost",
            "kind": "https",
        },
        "http_payload": {
            "name": "https.localhost",
            "policies": [
                {
                    "backend": f"http://127.0.0.1:{HTTP_BACKEND_PORT}",
                    "pathname_kind": "prefix",
                    "pathname": "/",
                }
            ],
        },
        "path": "/https-demo",
        "expect_tag": "https-ok",
    },
    {
        "name": "tls_terminate",
        "hostname": "terminate.localhost",
        "tls_payload": {
            "name": "terminate.localhost",
            "sni": "terminate.localhost",
            "kind": "tlsTerminate",
            "backend_hostname": "127.0.0.1",
            "backend_port": TERMINATE_BACKEND_PORT,
        },
        "http_payload": None,
        "path": "/terminate-demo",
        "expect_tag": "terminate-ok",
    },
    {
        "name": "tls_passthrough",
        "hostname": "passthrough.localhost",
        "tls_payload": {
            "name": "passthrough.localhost",
            "sni": "passthrough.localhost",
            "kind": "tlsPassthrough",
            "backend_hostname": "127.0.0.1",
            "backend_port": 8003,
        },
        "http_payload": None,
        "path": "/passthrough-demo",
        "expect_host": "passthrough.localhost",
    },
]


def publish_tls(hostname: str, payload: dict[str, Any]) -> None:
    response = requests.put(
        f"{MASTER_BASE}/api/master/tls",
        params={"hostname": hostname},
        headers={"Content-Type": "application/json"},
        data=json.dumps(payload),
        timeout=5,
    )
    response.raise_for_status()


def publish_http(hostname: str, payload: dict[str, Any]) -> None:
    response = requests.put(
        f"{MASTER_BASE}/api/master/http",
        params={"hostname": hostname},
        headers={"Content-Type": "application/json"},
        data=json.dumps(payload),
        timeout=5,
    )
    response.raise_for_status()


def tls_request(server_name: str, path: str) -> dict[str, Any]:
    context = ssl.create_default_context()
    context.check_hostname = False
    context.verify_mode = ssl.CERT_NONE

    with socket.create_connection(TLS_FRONTEND, timeout=2) as sock:
        with context.wrap_socket(sock, server_hostname=server_name) as tls_sock:
            request = (
                f"GET {path} HTTP/1.1\r\n"
                f"Host: {server_name}\r\n"
                "Connection: close\r\n"
                "\r\n"
            )
            tls_sock.sendall(request.encode("utf-8"))
            response = b""
            while True:
                chunk = tls_sock.recv(4096)
                if not chunk:
                    break
                response += chunk

    header_blob, _, body = response.partition(b"\r\n\r\n")
    status_line = header_blob.splitlines()[0].decode("utf-8", errors="replace")
    if " 200 " not in status_line:
        raise AssertionError(f"unexpected tls response status: {status_line}")
    return json.loads(body.decode("utf-8"))


def assert_eventually(case: dict[str, Any], timeout: float = 5.0) -> None:
    deadline = time.time() + timeout
    last_error = "no response"
    while time.time() < deadline:
        try:
            data = tls_request(case["hostname"], case["path"])
            expect_tag = case.get("expect_tag")
            if expect_tag and data.get("tag") != expect_tag:
                last_error = f"tag={data.get('tag')!r}"
                time.sleep(0.2)
                continue
            expect_host = case.get("expect_host")
            if expect_host and data.get("host") != expect_host:
                last_error = f"host={data.get('host')!r}"
                time.sleep(0.2)
                continue
            if data.get("path") != case["path"]:
                last_error = f"path={data.get('path')!r}"
                time.sleep(0.2)
                continue
            return
        except Exception as exc:  # noqa: BLE001
            last_error = str(exc)
            time.sleep(0.2)
    raise AssertionError(f"TLS case {case['name']} failed: {last_error}")


def ensure_compatible_self_signed_cert(cert_dir: pathlib.Path, hosts: list[str]) -> tuple[pathlib.Path, pathlib.Path]:
    try:
        return ensure_self_signed_cert(cert_dir, hosts)
    except subprocess.CalledProcessError:
        cert_file = cert_dir / "localhost-cert.pem"
        key_file = cert_dir / "localhost-key.pem"
        config_file = cert_dir / "openssl-san.cnf"
        san_lines = [f"DNS.{index + 1} = {host}" for index, host in enumerate(hosts)]
        san_lines.append(f"DNS.{len(san_lines) + 1} = localhost")
        san_lines.append("IP.1 = 127.0.0.1")
        config_file.write_text(
            "\n".join(
                [
                    "[req]",
                    "distinguished_name = req_distinguished_name",
                    "x509_extensions = v3_req",
                    "prompt = no",
                    "",
                    "[req_distinguished_name]",
                    "CN = localhost",
                    "",
                    "[v3_req]",
                    "subjectAltName = @alt_names",
                    "",
                    "[alt_names]",
                    *san_lines,
                    "",
                ]
            ),
            encoding="utf-8",
        )
        cmd = [
            "openssl",
            "req",
            "-x509",
            "-nodes",
            "-newkey",
            "rsa:2048",
            "-keyout",
            str(key_file),
            "-out",
            str(cert_file),
            "-days",
            "365",
            "-config",
            str(config_file),
        ]
        subprocess.run(cmd, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        return cert_file, key_file


def main() -> int:
    http_backend = start_http_backend(HTTP_BACKEND_PORT, "https-ok")
    terminate_backend = start_http_backend(TERMINATE_BACKEND_PORT, "terminate-ok")

    hosts = [case["hostname"] for case in TLS_CASES]
    cert_dir = pathlib.Path(tempfile.gettempdir()) / "venus-edge-test-certs"
    cert_dir.mkdir(parents=True, exist_ok=True)
    cert_file, key_file = ensure_compatible_self_signed_cert(cert_dir, hosts)
    https_backend = serve_https(cert_file, key_file)
    https_backend.tag = "passthrough-ok"

    cert_pem = cert_file.read_text(encoding="utf-8")
    key_pem = key_file.read_text(encoding="utf-8")

    try:
        for case in TLS_CASES:
            tls_payload = dict(case["tls_payload"])
            tls_payload["cert_pem"] = cert_pem
            tls_payload["key_pem"] = key_pem
            publish_tls(case["hostname"], tls_payload)
            if case["http_payload"] is not None:
                publish_http(case["hostname"], case["http_payload"])
            assert_eventually(case)
            print(f"ok case={case['name']}")
        return 0
    finally:
        http_backend.shutdown()
        terminate_backend.shutdown()
        https_backend.shutdown()


if __name__ == "__main__":
    raise SystemExit(main())
