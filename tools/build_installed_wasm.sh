#!/bin/sh
# Build-time operation only. The server never compiles source or accepts uploads.
set -eu
cd "$(dirname "$0")/.."
for name in shared_targets between filtered; do
  GOOS=wasip1 GOARCH=wasm CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags=-buildid= \
    -o "remote/installedwasm/assets/$name.wasm" "./examples/wasm/$name"
  chmod 644 "remote/installedwasm/assets/$name.wasm"
done
