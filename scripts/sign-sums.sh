#!/usr/bin/env bash
# Signs SHA256SUMS for the in-app updater: writes SHA256SUMS.sig (raw ed25519)
# next to it, then checks it against the public key compiled into the app
# (wails/internal/update/verify.go), so a mismatched secret fails the release
# instead of shipping updates no installed copy would accept.
#
#   UPDATE_SIGNING_KEY=<PEM> scripts/sign-sums.sh dist/SHA256SUMS
set -euo pipefail

sums=${1:?usage: sign-sums.sh <SHA256SUMS>}
root=$(cd "$(dirname "$0")/.." && pwd)
[[ -n "${UPDATE_SIGNING_KEY:-}" ]] || { echo "::error::UPDATE_SIGNING_KEY is not set" >&2; exit 1; }

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
umask 077
printf '%s\n' "$UPDATE_SIGNING_KEY" > "$tmp/key.pem"
openssl pkeyutl -sign -inkey "$tmp/key.pem" -rawin -in "$sums" -out "$sums.sig"

# The embedded key is 32 raw bytes; prefix the ed25519 SubjectPublicKeyInfo header for OpenSSL.
pub=$(sed -n 's/^var PublicKey = mustKey("\([^"]*\)")$/\1/p' "$root/wails/internal/update/verify.go")
[[ -n "$pub" ]] || { echo "::error::no PublicKey in verify.go" >&2; exit 1; }
{ printf '\x30\x2a\x30\x05\x06\x03\x2b\x65\x70\x03\x21\x00'; base64 -d <<<"$pub"; } > "$tmp/pub.der"
openssl pkeyutl -verify -pubin -keyform DER -inkey "$tmp/pub.der" -rawin -in "$sums" -sigfile "$sums.sig" >/dev/null \
  || { echo "::error::SHA256SUMS.sig does not verify with the app's PublicKey: wrong UPDATE_SIGNING_KEY?" >&2; exit 1; }
echo "signed $sums"
