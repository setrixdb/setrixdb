#!/usr/bin/env bash
# Copyright (c) 2026 SetrixDB
# SPDX-License-Identifier: Apache-2.0
#
# Build da C ABI (c-shared) e teste de ponta a ponta em C e Python.
set -euo pipefail
cd "$(dirname "$0")"

echo "[1/3] build libsetrixdb.so"
CGO_ENABLED=1 go build -buildmode=c-shared -o ./libsetrixdb.so ../../cmd/setrixdb-capi

echo "[2/3] teste em C"
gcc test.c -I. -L. -lsetrixdb -o /tmp/setrixdb-capitest
LD_LIBRARY_PATH=. /tmp/setrixdb-capitest

echo "[3/3] teste em Python (ctypes)"
python3 test.py

echo "OK"
