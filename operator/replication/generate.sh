#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
proto="${script_dir}/replication.proto"

gopath="$(go env GOPATH)"
protoc_gen_go="${GOBIN:-${gopath}/bin}/protoc-gen-go"
protoc_gen_go_grpc="${GOBIN:-${gopath}/bin}/protoc-gen-go-grpc"

if [[ ! -x "${protoc_gen_go}" ]]; then
  echo "protoc-gen-go not found at ${protoc_gen_go}" >&2
  exit 1
fi

if [[ ! -x "${protoc_gen_go_grpc}" ]]; then
  echo "protoc-gen-go-grpc not found at ${protoc_gen_go_grpc}" >&2
  exit 1
fi

PATH="$(dirname "${protoc_gen_go}"):${PATH}"

protoc \
  --proto_path="${script_dir}" \
  --go_out="${script_dir}" \
  --go_opt=paths=source_relative \
  --go-grpc_out="${script_dir}" \
  --go-grpc_opt=paths=source_relative \
  "${proto}"
