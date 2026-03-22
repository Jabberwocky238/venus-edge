#!/usr/bin/env bash

set -euo pipefail

# map
declare -A mapping=(
  [ingress]="ingress/schema/ingress.capnp"
  [wal]="operator/replication/wal.capnp"
  [DNS]="DNS/schema/dns.capnp"
)

caller="${1:-}"

if [[ -z "${caller}" ]]; then
  echo "usage: $0 <ingress|wal|DNS>" >&2
  exit 1
fi

if [[ -z "${mapping[$caller]:-}" ]]; then
  echo "unknown schema target: ${caller}" >&2
  echo "available: ingress wal DNS" >&2
  exit 1
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
schema="${script_dir}/${mapping[$caller]}"
output_dir="${script_dir}"

gopath="$(go env GOPATH)"
capnpc_go="${GOBIN:-${gopath}/bin}/capnpc-go"
capnp_std="${gopath}/pkg/mod/capnproto.org/go/capnp/v3@v3.1.0-alpha.2/std"

if [[ ! -x "${capnpc_go}" ]]; then
  echo "capnpc-go not found at ${capnpc_go}" >&2
  exit 1
fi

if [[ ! -d "${capnp_std}" ]]; then
  echo "Cap'n Proto Go std schema dir not found at ${capnp_std}" >&2
  exit 1
fi

PATH="$(dirname "${capnpc_go}"):${PATH}"

capnp compile \
  -I "${capnp_std}" \
  -o go:"${output_dir}" \
  "${schema}"
