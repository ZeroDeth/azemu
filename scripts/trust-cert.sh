#!/usr/bin/env bash
# trust-cert.sh -- add the azemu self-signed TLS certificate to the system
# trust store. This is OPTIONAL; the default path uses SSL_CERT_FILE instead.
#
# Usage:
#   ./scripts/trust-cert.sh [path-to-cert-bundle]
#
# If no path is given, defaults to .azemu/cert-bundle.pem relative to the
# repository root.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

CERT="${1:-${REPO_ROOT}/.azemu/cert-bundle.pem}"

if [ ! -f "${CERT}" ]; then
  echo "trust-cert: error: cert not found at ${CERT}" >&2
  echo "trust-cert: start azemu first: docker compose up -d --build" >&2
  exit 1
fi

OS="$(uname -s)"
case "${OS}" in
  Darwin)
    echo "trust-cert: adding cert to macOS login keychain..." >&2
    security add-trusted-cert \
      -r trustRoot \
      -p ssl \
      -k ~/Library/Keychains/login.keychain-db \
      "${CERT}"
    echo "trust-cert: done. The cert is now trusted system-wide on macOS." >&2
    ;;
  Linux)
    echo "trust-cert: copying cert to /usr/local/share/ca-certificates/..." >&2
    sudo cp "${CERT}" /usr/local/share/ca-certificates/azemu.crt
    sudo update-ca-certificates
    echo "trust-cert: done. The cert is now trusted system-wide on Linux." >&2
    ;;
  *)
    echo "trust-cert: error: unsupported OS '${OS}'" >&2
    echo "trust-cert: manually add ${CERT} to your system trust store." >&2
    exit 1
    ;;
esac
