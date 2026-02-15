#!/bin/bash
# Build the Swift Secure Enclave code into a static library.
# Called by the Makefile before go build with CGO_ENABLED=1.
set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUT_DIR="${SCRIPT_DIR}"

SWIFT_FILE="${SCRIPT_DIR}/enclave.swift"
OBJ_FILE="${OUT_DIR}/enclave_swift.o"
LIB_FILE="${OUT_DIR}/libenclave.a"

# Get the macOS deployment target
MACOS_TARGET="${MACOS_DEPLOYMENT_TARGET:-11.0}"

# Determine architecture
ARCH="$(uname -m)"

echo "Compiling enclave.swift -> libenclave.a (${ARCH}, macOS ${MACOS_TARGET})"

swiftc \
    -emit-object \
    -parse-as-library \
    -o "${OBJ_FILE}" \
    -target "${ARCH}-apple-macosx${MACOS_TARGET}" \
    -O \
    "${SWIFT_FILE}"

# Create a static library from the object file
ar rcs "${LIB_FILE}" "${OBJ_FILE}"
rm -f "${OBJ_FILE}"

echo "Built: ${LIB_FILE}"
