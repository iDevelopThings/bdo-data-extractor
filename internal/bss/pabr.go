package bss

import "fmt"

// PABR is the validated header of a PABR table: [PABR][u32 Rows][fixed or
// variable records …][string table], where a u64 at the last 8 bytes points at
// the string table. The records occupy [RecordsStart, StringTablePos).
type PABR struct {
	Rows           int
	RecordsStart   int
	StringTablePos int
}

// OpenPABR validates the PABR framing and returns its header. It does not read
// the string table — callers choose raw vs UTF-16 via ReadStringTable /
// ReadUTF16StringTable at StringTablePos.
func OpenPABR(data []byte) (PABR, error) {
	if len(data) < 16 {
		return PABR{}, fmt.Errorf("not a PABR table")
	}
	rows, ok := PABRCount(data)
	if !ok {
		return PABR{}, fmt.Errorf("not a PABR table")
	}
	stPos := int(U64(data, len(data)-8))
	if stPos <= 8 || stPos > len(data) {
		return PABR{}, fmt.Errorf("bad PABR header (rows=%d stringTablePos=%d)", rows, stPos)
	}
	return PABR{Rows: rows, RecordsStart: 8, StringTablePos: stPos}, nil
}

// PABRCount returns the u32 at offset 4 when data begins with the PABR magic.
// Prefer OpenPABR for full tables with a string-table footer; use this for
// PABR-framed blobs that are not full tables (BKD/RID sidecars).
func PABRCount(data []byte) (rows int, ok bool) {
	if len(data) < 8 || string(data[:4]) != "PABR" {
		return 0, false
	}
	rows = int(U32(data, 4))
	return rows, rows > 0
}

// RecordSize returns the fixed record width for tables whose records evenly tile
// [RecordsStart, StringTablePos); ok is false when they don't divide evenly.
func (p PABR) RecordSize() (size int, ok bool) {
	span := p.StringTablePos - p.RecordsStart
	if p.Rows == 0 || span%p.Rows != 0 {
		return 0, false
	}
	return span / p.Rows, true
}
