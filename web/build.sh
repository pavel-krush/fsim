#!/bin/sh
# Builds the browser version into web/. Two artefacts, both generated:
#
#   fsim.wasm      the simulator, ~18 MB, or ~5 MB over gzip
#   wasm_exec.js   Go's own loader, copied out of GOROOT so that it always
#                  matches the toolchain the wasm was built with
#
# Serve the directory over HTTP — a file:// page cannot fetch the wasm:
#
#   web/build.sh && (cd web && python3 -m http.server 8080)
set -e
cd "$(dirname "$0")/.."
GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o web/fsim.wasm .
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/wasm_exec.js
ls -la web/fsim.wasm web/wasm_exec.js
