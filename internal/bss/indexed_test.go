package bss

import (
	"encoding/binary"
	"errors"
	"testing"
)

func make12Index(entries ...IndexEntry) []byte {
	b := binary.LittleEndian.AppendUint32(nil, uint32(len(entries)))
	for _, e := range entries {
		b = binary.LittleEndian.AppendUint32(b, e.Key)
		b = binary.LittleEndian.AppendUint32(b, e.Offset)
		b = binary.LittleEndian.AppendUint32(b, e.Size)
	}
	return b
}

func TestIndexedRecordsYieldsSlices(t *testing.T) {
	t.Parallel()

	data := []byte{
		1, 2, 3, 4,
		5, 6, 7, 8,
	}
	index := make12Index(
		IndexEntry{Key: 10, Offset: 0, Size: 4},
		IndexEntry{Key: 20, Offset: 4, Size: 4},
	)

	var keys []uint32
	var sizes []int
	for rec, err := range IndexedRecords(index, data) {
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, rec.Entry.Key)
		sizes = append(sizes, len(rec.Data))
	}
	if len(keys) != 2 || keys[0] != 10 || keys[1] != 20 {
		t.Fatalf("keys = %v", keys)
	}
	if sizes[0] != 4 || sizes[1] != 4 {
		t.Fatalf("sizes = %v", sizes)
	}
}

func TestIndexedRecordsStopsOnParseError(t *testing.T) {
	t.Parallel()

	n := 0
	for _, err := range IndexedRecords([]byte{1, 2}, nil) {
		n++
		if err == nil {
			t.Fatal("expected parse error")
		}
	}
	if n != 1 {
		t.Fatalf("yielded %d times, want 1", n)
	}
}

func TestRecordsFromEntriesStopsOnOutOfBoundsSlice(t *testing.T) {
	t.Parallel()

	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	entries := []IndexEntry{
		{Key: 1, Offset: 0, Size: 4},
		{Key: 2, Offset: 4, Size: 99},
	}
	n := 0
	var gotErr error
	for rec, err := range RecordsFromEntries(entries, data) {
		n++
		if err != nil {
			gotErr = err
			if rec.Entry.Key != 2 {
				t.Fatalf("error entry key = %d, want 2", rec.Entry.Key)
			}
			break
		}
		if rec.Entry.Key != 1 {
			t.Fatalf("first key = %d, want 1", rec.Entry.Key)
		}
	}
	if gotErr == nil {
		t.Fatal("expected out-of-bounds error")
	}
	if n != 2 {
		t.Fatalf("yielded %d times, want 2 (ok then err)", n)
	}
}

func TestIndexedRecordsU16(t *testing.T) {
	t.Parallel()

	data := []byte{9, 9, 9, 9, 1, 2, 3, 4}
	index := binary.LittleEndian.AppendUint32(nil, 1)
	index = binary.LittleEndian.AppendUint16(index, 42)
	index = binary.LittleEndian.AppendUint32(index, 4)
	index = binary.LittleEndian.AppendUint32(index, 4)

	var got IndexedRecord
	for rec, err := range IndexedRecordsU16("test", index, data) {
		if err != nil {
			t.Fatal(err)
		}
		got = rec
	}
	if got.Entry.Key != 42 || len(got.Data) != 4 || got.Data[0] != 1 {
		t.Fatalf("got %+v data=%v", got.Entry, got.Data)
	}
}

func TestRequireExhausted(t *testing.T) {
	t.Parallel()

	ok := NewCursor([]byte{1, 2, 3, 4}, 0, 4)
	ok.U32()
	if err := RequireExhausted(ok); err != nil {
		t.Fatal(err)
	}

	trail := NewCursor([]byte{1, 2, 3, 4}, 0, 4)
	trail.U16()
	if err := RequireExhausted(trail); err == nil {
		t.Fatal("expected trailing-bytes error")
	}

	short := NewCursor([]byte{1, 2}, 0, 2)
	short.U32()
	if err := RequireExhausted(short); !errors.Is(err, ErrTruncated) {
		t.Fatalf("err = %v, want ErrTruncated", err)
	}
}
