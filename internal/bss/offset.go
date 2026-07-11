package bss

import (
	"fmt"
	"sort"
)

// IndexEntry is one row of a PABR *offset.dbss index: a key (usually an item id)
// pointing at a byte slice [Offset, Offset+Size) in the paired data file.
type IndexEntry struct {
	Key    uint32
	Offset uint32
	Size   uint32
}

// Slice returns the record bytes [Offset, Offset+Size) within data, or ok=false
// if they fall outside it. Table-specific length/key checks are the caller's.
func (e IndexEntry) Slice(data []byte) (rec []byte, ok bool) {
	end := int(e.Offset) + int(e.Size)
	if end < int(e.Offset) || end > len(data) {
		return nil, false
	}
	return data[e.Offset:end], true
}

// ParseOffsetIndex decodes a PABR offset index (12-byte records of three u32
// columns: key, offset and size in some order). The roles are detected by
// content against dataLen: the offset/size pairing is the one whose
// [offset, offset+size) intervals all fit in the data and don't overlap; ties
// (e.g. sparse small keys) break toward the pairing that tiles the data most
// tightly. Works whether the index is sorted by offset or by key.
func ParseOffsetIndex(b []byte, dataLen int) ([]IndexEntry, error) {
	if len(b) < 8 {
		return nil, fmt.Errorf("offset index too small")
	}
	hdr := 4
	cnt := U32(b, 0)
	if string(b[0:4]) == "PABR" {
		hdr = 8
		cnt = U32(b, 4)
	}
	if cnt == 0 || hdr+int(cnt)*12 > len(b) {
		return nil, fmt.Errorf("bad offset-index count %d", cnt)
	}
	col := func(c int) func(i int) uint32 {
		return func(i int) uint32 { return U32(b, hdr+i*12+c*4) }
	}
	cols := [3]func(int) uint32{col(0), col(1), col(2)}

	// eval returns the "gap" (uncovered bytes) for an offset/size pairing, and
	// whether it's a valid tiling: every record in bounds and non-overlapping.
	eval := func(oc, sc int) (uint64, bool) {
		type iv struct{ off, end uint32 }
		ivs := make([]iv, cnt)
		var sum uint64
		for i := 0; i < int(cnt); i++ {
			o, s := cols[oc](i), cols[sc](i)
			if s == 0 || int(o)+int(s) > dataLen {
				return 0, false
			}
			ivs[i] = iv{o, o + s}
			sum += uint64(s)
		}
		sort.Slice(ivs, func(a, c int) bool { return ivs[a].off < ivs[c].off })
		for i := 1; i < len(ivs); i++ {
			if ivs[i].off < ivs[i-1].end {
				return 0, false
			}
		}
		span := uint64(ivs[len(ivs)-1].end - ivs[0].off)
		return span - sum, true
	}

	bestGap, oc, sc := ^uint64(0), -1, -1
	for a := 0; a < 3; a++ {
		for s := 0; s < 3; s++ {
			if a == s {
				continue
			}
			if gap, ok := eval(a, s); ok && gap < bestGap {
				bestGap, oc, sc = gap, a, s
			}
		}
	}
	if oc == -1 {
		return nil, fmt.Errorf("could not detect offset/size columns")
	}

	kc := 3 - oc - sc
	out := make([]IndexEntry, cnt)
	for i := range out {
		out[i] = IndexEntry{Key: cols[kc](i), Offset: cols[oc](i), Size: cols[sc](i)}
	}
	return out, nil
}
