package update

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

// PublicKey checks SHA256SUMS.sig. The private half is the release workflow's
// UPDATE_SIGNING_KEY secret (an ed25519 PEM key, `openssl pkeyutl -sign
// -rawin`). Replacing it means old builds can no longer update themselves.
var PublicKey = mustKey("P9XUC2KyxmRCCCKc/c1R6Q0+VZnL8GV18gGVbZsyN2I=")

func mustKey(b64 string) ed25519.PublicKey {
	k, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(k) != ed25519.PublicKeySize {
		panic("update: bad public key")
	}
	return k
}

// ErrUnverified means the checksums are not signed by PublicKey, or a file
// does not match them.
var ErrUnverified = errors.New("update_unverified")

// VerifySums checks the signature of a SHA256SUMS file and returns its
// entries (file name → lower-case hex SHA-256).
func VerifySums(sums, sig []byte, pub ed25519.PublicKey) (map[string]string, error) {
	if len(sig) != ed25519.SignatureSize || !ed25519.Verify(pub, sums, sig) {
		return nil, ErrUnverified
	}
	return parseSums(sums), nil
}

// parseSums reads `sha256sum` output: "<hex>  <name>" or "<hex> *<name>".
// Malformed lines are skipped; a name listed twice is dropped, so an entry
// can never be ambiguous.
func parseSums(b []byte) map[string]string {
	out := map[string]string{}
	seen := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		hash, name, ok := strings.Cut(sc.Text(), " ")
		if !ok || len(hash) != sha256.Size*2 {
			continue
		}
		if _, err := hex.DecodeString(hash); err != nil {
			continue
		}
		name = strings.TrimPrefix(strings.TrimPrefix(name, " "), "*")
		if name == "" || strings.ContainsAny(name, "/\\") {
			continue
		}
		if seen[name] {
			delete(out, name)
			continue
		}
		seen[name] = true
		out[name] = strings.ToLower(hash)
	}
	return out
}
