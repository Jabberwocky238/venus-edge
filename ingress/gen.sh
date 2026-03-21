#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
schema="${script_dir}/ingress.capnp"
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
