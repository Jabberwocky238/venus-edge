#!/usr/bin/env python3
from __future__ import annotations

import http.server
import os
import pathlib
import socketserver
import ssl
import subprocess
import sys
import tempfile
import threading


HTTP_PORT = 8000
HTTPS_PORT = 8003
DEFAULT_HOSTS = [
    "a.localhost",
    "b.localhost",
    "api.localhost",
]


class ThreadingHTTPServer(socketserver.ThreadingMixIn, http.server.HTTPServer):
    daemon_threads = True
    allow_reuse_address = True


class MultiHostHandler(http.server.BaseHTTPRequestHandler):
    server_version = "MultiHostTestServer/1.0"

    def do_GET(self) -> None:
        host = self.headers.get("Host", "")
        normalized_host = host.split(":", 1)[0].strip().lower()
        scheme = "https" if isinstance(self.connection, ssl.SSLSocket) else "http"

        body = (
            "{\n"
            f'  "scheme": "{scheme}",\n'
            f'  "host": "{normalized_host}",\n'
            f'  "port": {self.server.server_port},\n'
            f'  "path": "{self.path}"\n'
            "}\n"
        )

        self.send_response(200)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body.encode("utf-8"))))
        self.end_headers()
        self.wfile.write(body.encode("utf-8"))

    def log_message(self, fmt: str, *args: object) -> None:
        sys.stderr.write(
            "[%s] %s - - %s\n"
            % (
                "HTTPS" if isinstance(self.connection, ssl.SSLSocket) else "HTTP",
                self.address_string(),
                fmt % args,
            )
        )


def ensure_self_signed_cert(cert_dir: pathlib.Path, hosts: list[str]) -> tuple[pathlib.Path, pathlib.Path]:
    cert_file = cert_dir / "localhost-cert.pem"
    key_file = cert_dir / "localhost-key.pem"
    if cert_file.exists() and key_file.exists():
        return cert_file, key_file

    san_entries = ",".join(f"DNS:{host}" for host in hosts) + ",DNS:localhost,IP:127.0.0.1"
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
        "-subj",
        "/CN=localhost",
        "-addext",
        f"subjectAltName={san_entries}",
    ]
    try:
        subprocess.run(cmd, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    except FileNotFoundError as exc:
        raise RuntimeError("openssl is required to generate the self-signed certificate") from exc
    return cert_file, key_file


def serve_http() -> ThreadingHTTPServer:
    server = ThreadingHTTPServer(("0.0.0.0", HTTP_PORT), MultiHostHandler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    return server


def serve_https(cert_file: pathlib.Path, key_file: pathlib.Path) -> ThreadingHTTPServer:
    server = ThreadingHTTPServer(("0.0.0.0", HTTPS_PORT), MultiHostHandler)
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain(certfile=str(cert_file), keyfile=str(key_file))
    server.socket = context.wrap_socket(server.socket, server_side=True)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    return server


def main() -> int:
    hosts = os.environ.get("TEST_SERVER_HOSTS")
    domains = [h.strip() for h in hosts.split(",")] if hosts else DEFAULT_HOSTS
    domains = [h for h in domains if h]

    cert_dir = pathlib.Path(tempfile.gettempdir()) / "venus-edge-test-certs"
    cert_dir.mkdir(parents=True, exist_ok=True)
    cert_file, key_file = ensure_self_signed_cert(cert_dir, domains)

    http_server = serve_http()
    https_server = serve_https(cert_file, key_file)

    print(f"HTTP  server listening on :{HTTP_PORT}")
    print(f"HTTPS server listening on :{HTTPS_PORT}")
    print("Configured hostnames:")
    for domain in domains:
        print(f"  - {domain}")
    print("")
    print("Examples:")
    print(f"  curl -H 'Host: {domains[0]}' http://127.0.0.1:{HTTP_PORT}/")
    print(f"  curl -k -H 'Host: {domains[0]}' https://127.0.0.1:{HTTPS_PORT}/")

    try:
        threading.Event().wait()
    except KeyboardInterrupt:
        pass
    finally:
        http_server.shutdown()
        https_server.shutdown()

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
