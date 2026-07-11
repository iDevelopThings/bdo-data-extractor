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
	if len(data) < 16 || string(data[:4]) != "PABR" {
		return PABR{}, fmt.Errorf("not a PABR table")
	}
	rows := int(U32(data, 4))
	stPos := int(U64(data, len(data)-8))
	if rows <= 0 || stPos <= 8 || stPos > len(data) {
		return PABR{}, fmt.Errorf("bad PABR header (rows=%d stringTablePos=%d)", rows, stPos)
	}
	return PABR{Rows: rows, RecordsStart: 8, StringTablePos: stPos}, nil
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
