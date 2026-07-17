package paz

import (
	"bytes"
	"testing"
)

func TestDecodeInnerPABR(t *testing.T) {
	t.Parallel()

	plain := []byte{'P', 'A', 'B', 'R', 1, 0, 0, 0, 8, 0, 0, 0, 0, 0, 0, 0}
	encrypted := NewICE(BDOICEKey).Encrypt(bytes.Clone(plain))
	encryptedInput := bytes.Clone(encrypted)
	if got := decodeInnerPABR(encrypted); !bytes.Equal(got, plain) {
		t.Fatalf("decoded bytes = %x, want %x", got, plain)
	}
	if !bytes.Equal(encrypted, encryptedInput) {
		t.Fatalf("encrypted input mutated: %x", encrypted)
	}

	nonPABR := []byte("0123456789abcdef")
	expected := bytes.Clone(nonPABR)
	if got := decodeInnerPABR(nonPABR); !bytes.Equal(got, expected) {
		t.Fatalf("non-PABR bytes changed: %x", got)
	}
	if !bytes.Equal(nonPABR, expected) {
		t.Fatalf("non-PABR input mutated: %x", nonPABR)
	}
}
