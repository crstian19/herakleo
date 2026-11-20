package update

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
)

const (
	hashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	hashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func newKey(t testing.TB) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func TestVerifySums(t *testing.T) {
	pub, priv := newKey(t)
	sums := []byte(hashA + "  herakleo-0.5.0-linux-x86_64.tar.gz\n" + hashB + " *herakleo.exe\n")
	sig := ed25519.Sign(priv, sums)

	got, err := VerifySums(sums, sig, pub)
	if err != nil {
		t.Fatal(err)
	}
	if got["herakleo-0.5.0-linux-x86_64.tar.gz"] != hashA || got["herakleo.exe"] != hashB {
		t.Fatalf("got %v", got)
	}

	otherPub, _ := newKey(t)
	tampered := append([]byte{}, sums...)
	tampered[0] = 'c'
	for name, c := range map[string]struct {
		sums, sig []byte
		pub       ed25519.PublicKey
	}{
		"tampered sums": {tampered, sig, pub},
		"other key":     {sums, sig, otherPub},
		"short sig":     {sums, sig[:10], pub},
		"no sig":        {sums, nil, pub},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := VerifySums(c.sums, c.sig, c.pub); !errors.Is(err, ErrUnverified) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestParseSums(t *testing.T) {
	in := strings.Join([]string{
		hashA + "  good",
		hashA + "  dup",
		hashB + "  dup",
		"short  bad",
		"zz" + hashA[2:] + "  nothex",
		hashA + "  ../escape",
		hashA + "  dir\\file",
		"",
	}, "\n")
	got := parseSums([]byte(in))
	if len(got) != 1 || got["good"] != hashA {
		t.Fatalf("got %v", got)
	}
}

func TestEmbeddedKeyIsValid(t *testing.T) {
	if len(PublicKey) != ed25519.PublicKeySize {
		t.Fatal("bad key size")
	}
}
