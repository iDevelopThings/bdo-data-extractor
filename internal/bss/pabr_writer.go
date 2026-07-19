package bss

import "encoding/binary"

// PackPABR builds a minimal PABR blob: magic, row count, record bytes, an empty
// string table, and the trailing string-table pointer. Inverse of OpenPABR for
// tests and probes — keep write helpers here rather than beside the reader.
func PackPABR(rows int, records []byte) []byte {
	out := make([]byte, 0, 8+len(records)+4+8)
	out = append(out, 'P', 'A', 'B', 'R')
	out = binary.LittleEndian.AppendUint32(out, uint32(rows))
	out = append(out, records...)
	stPos := len(out)
	out = binary.LittleEndian.AppendUint32(out, 0) // empty string-table count
	out = binary.LittleEndian.AppendUint64(out, uint64(stPos))
	return out
}
