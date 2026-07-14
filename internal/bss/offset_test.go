package bss

import (
	"encoding/binary"
	"testing"
)

func appendRow(index []byte, e IndexEntry) []byte {
	index = binary.LittleEndian.AppendUint16(index, uint16(e.Key))
	index = binary.LittleEndian.AppendUint32(index, e.Offset)
	index = binary.LittleEndian.AppendUint32(index, e.Size)
	return index
}

func TestParseU16OffsetIndex(t *testing.T) {
	index := []byte{'P', 'A', 'B', 'R', 2, 0, 0, 0}
	index = appendRow(index, IndexEntry{Key: 20, Offset: 8, Size: 4})
	index = appendRow(index, IndexEntry{Key: 10, Offset: 4, Size: 4})

	entries, err := ParseU16OffsetIndex("test", index, 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0] != (IndexEntry{Key: 20, Offset: 8, Size: 4}) {
		t.Fatalf("first entry = %+v", entries[0])
	}
	if entries[1] != (IndexEntry{Key: 10, Offset: 4, Size: 4}) {
		t.Fatalf("second entry = %+v", entries[1])
	}
}

// One unusable row must not take the rest of the table down with it — a game
// patch adding an odd row should cost that row, not every other row.
func TestParseU16OffsetIndexSkipsOverlappingRow(t *testing.T) {
	index := binary.LittleEndian.AppendUint32(nil, 3)
	index = appendRow(index, IndexEntry{Key: 1, Offset: 0, Size: 4})
	index = appendRow(index, IndexEntry{Key: 2, Offset: 2, Size: 4}) // overlaps key 1
	index = appendRow(index, IndexEntry{Key: 3, Offset: 8, Size: 4})

	entries, err := ParseU16OffsetIndex("test", index, 12)
	if err != nil {
		t.Fatalf("overlapping row should be skipped, not fail the table: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2 (the overlapping row dropped)", len(entries))
	}
	if entries[0].Key != 1 || entries[1].Key != 3 {
		t.Fatalf("kept keys = %d, %d; want 1, 3", entries[0].Key, entries[1].Key)
	}
}

func TestParseU16OffsetIndexSkipsZeroSizeAndOutOfBoundsRows(t *testing.T) {
	index := binary.LittleEndian.AppendUint32(nil, 3)
	index = appendRow(index, IndexEntry{Key: 1, Offset: 0, Size: 0})  // zero size
	index = appendRow(index, IndexEntry{Key: 2, Offset: 8, Size: 99}) // past end of data
	index = appendRow(index, IndexEntry{Key: 3, Offset: 4, Size: 4})

	entries, err := ParseU16OffsetIndex("test", index, 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Key != 3 {
		t.Fatalf("kept key = %d, want 3", entries[0].Key)
	}
}

// An index where nothing survives is still an error: shipping a silently empty
// table is worse than failing the build.
func TestParseU16OffsetIndexErrorsWhenNoRowSurvives(t *testing.T) {
	index := binary.LittleEndian.AppendUint32(nil, 2)
	index = appendRow(index, IndexEntry{Key: 1, Offset: 0, Size: 0})
	index = appendRow(index, IndexEntry{Key: 2, Offset: 40, Size: 4})

	if _, err := ParseU16OffsetIndex("test", index, 12); err == nil {
		t.Fatal("expected an error when no row is usable")
	}
}

func TestParseU16OffsetIndexErrorsOnBadHeader(t *testing.T) {
	if _, err := ParseU16OffsetIndex("test", []byte{1, 2}, 12); err == nil {
		t.Fatal("expected an error for a truncated index")
	}

	index := binary.LittleEndian.AppendUint32(nil, 99) // count far exceeds the buffer
	if _, err := ParseU16OffsetIndex("test", index, 12); err == nil {
		t.Fatal("expected an error for a count that overruns the index")
	}
}
