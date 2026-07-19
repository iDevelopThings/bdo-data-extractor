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

func TestDecodeProcessingMastery(t *testing.T) {
	t.Parallel()

	// manufacturingstat uses PABR magic with rows@4==0; bracket count lives at @8.
	rec := make([]byte, 12)
	rec[0], rec[1], rec[2], rec[3] = 'P', 'A', 'B', 'R'
	binary.LittleEndian.PutUint32(rec[8:], 1)
	body := make([]byte, 16)
	binary.LittleEndian.PutUint32(body[0:], math.Float32bits(50))
	binary.LittleEndian.PutUint32(body[4:], 500_000) // 0.5
	binary.LittleEndian.PutUint32(body[8:], 2)
	data := append(rec, body...)

	got, err := DecodeProcessingMastery(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Mastery != 50 || got[0].ProcRate != 0.5 || got[0].Batch != 2 {
		t.Fatalf("got %#v", got)
	}
}

func TestDecodeProcessingMasteryRejectsNonPABR(t *testing.T) {
	t.Parallel()
	if _, err := DecodeProcessingMastery([]byte("nope")); err == nil {
		t.Fatal("expected error")
	}
}
