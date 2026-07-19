package tables

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
)

func TestDecodeCookingMastery(t *testing.T) {
	t.Parallel()

	rec := make([]byte, 0, 24)
	rec = binary.LittleEndian.AppendUint32(rec, math.Float32bits(100))
	for i := 0; i < 5; i++ {
		rec = binary.LittleEndian.AppendUint32(rec, uint32((i+1)*1000))
	}
	data := bss.PackPABR(1, rec)

	got, err := DecodeCookingMastery(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Mastery != 100 {
		t.Fatalf("got %#v", got)
	}
	if got[0].Rates[0] != 0.001 || got[0].Rates[4] != 0.005 {
		t.Fatalf("rates = %v", got[0].Rates)
	}
}

func TestDecodeCookingMasteryRejectsNonPABR(t *testing.T) {
	t.Parallel()
	if _, err := DecodeCookingMastery([]byte("nope")); err == nil {
		t.Fatal("expected error")
	}
}

func TestDecodeZonesRejectsNonPABR(t *testing.T) {
	t.Parallel()
	if _, err := DecodeZones([]byte("nope"), func(uint32) bool { return false }); err == nil {
		t.Fatal("expected error")
	}
}

func TestDecodeManufactureRejectsNonPABR(t *testing.T) {
	t.Parallel()
	if _, err := DecodeManufacture([]byte("nope")); err == nil {
		t.Fatal("expected error")
	}
}
