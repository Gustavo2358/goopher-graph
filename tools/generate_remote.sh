#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
# Official generators. protoc-gen-go 1.34.2 emits no unsafe code of our own.
# PYTHON must provide grpcio-tools. See docs/REMOTE.md for installation.
${PYTHON:-python3} -m grpc_tools.protoc -I. \
  --go_out=. --go_opt=paths=source_relative \
  --go-grpc_out=. --go-grpc_opt=paths=source_relative \
  --python_out=clients/python --grpc_python_out=clients/python remote/pb/service.proto
${PYTHON:-python3} -m grpc_tools.protoc -I. --python_out=clients/python ggpb/pb/result.proto
