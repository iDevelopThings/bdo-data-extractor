package bss

import (
	"fmt"
	"log"
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

// ParseU16OffsetIndex decodes an offset index whose records are
// [u16 key, u32 offset, u32 size]. Both plain [u32 count] and PABR
// [magic, u32 count] headers are accepted. name identifies the table in logs.
// An individually unusable row (zero size, out of bounds, overlapping) is logged
// and skipped, so one odd row from a patch can't drop the table; a malformed
// header, or an index where no row survives, is still an error.
func ParseU16OffsetIndex(name string, b []byte, dataLen int) ([]IndexEntry, error) {
	if len(b) < 4 {
		return nil, fmt.Errorf("%s: u16 offset index too small (%d bytes)", name, len(b))
	}
	header := 4
	count := int(U32(b, 0))
	if len(b) >= 8 && string(b[:4]) == "PABR" {
		header = 8
		count = int(U32(b, 4))
	}
	if count <= 0 || header+count*10 > len(b) {
		return nil, fmt.Errorf("%s: bad u16 offset-index count %d (index is %d bytes)", name, count, len(b))
	}

	out := make([]IndexEntry, 0, count)
	for i := range count {
		o := header + i*10
		entry := IndexEntry{
			Key:    uint32(U16(b, o)),
			Offset: U32(b, o+2),
			Size:   U32(b, o+6),
		}
		switch {
		case entry.Size == 0:
			log.Printf("%s: offset-index row %d (key %d) has zero size — skipping row", name, i, entry.Key)
		case uint64(entry.Offset)+uint64(entry.Size) > uint64(dataLen):
			log.Printf("%s: offset-index row %d (key %d) is out of bounds (offset %d + size %d > data %d) — skipping row",
				name, i, entry.Key, entry.Offset, entry.Size, dataLen)
		default:
			out = append(out, entry)
		}
	}

	out = dropOverlappingRows(name, out)

	if len(out) == 0 {
		return nil, fmt.Errorf("%s: offset index declares %d rows, none usable", name, count)
	}
	return out, nil
}

// ParseU8OneBasedOffsetIndex decodes an index whose rows are
// [u8 key, u32 oneBasedOffset, u32 sizeMinusOne]. The returned entries use
// ordinary zero-based offsets and byte sizes, matching IndexEntry.Slice.
func ParseU8OneBasedOffsetIndex(name string, b []byte, dataLen int) ([]IndexEntry, error) {
	if len(b) < 4 {
		return nil, fmt.Errorf("%s: u8 offset index too small (%d bytes)", name, len(b))
	}
	count := int(U32(b, 0))
	if count <= 0 || 4+count*9 > len(b) {
		return nil, fmt.Errorf("%s: bad u8 offset-index count %d (index is %d bytes)", name, count, len(b))
	}

	out := make([]IndexEntry, 0, count)
	for i := range count {
		o := 4 + i*9
		storedOffset := U32(b, o+1)
		if storedOffset == 0 {
			log.Printf("%s: offset-index row %d (key %d) has a zero one-based offset — skipping row", name, i, b[o])
			continue
		}
		entry := IndexEntry{
			Key:    uint32(b[o]),
			Offset: storedOffset - 1,
			Size:   U32(b, o+5) + 1,
		}
		if uint64(entry.Offset)+uint64(entry.Size) > uint64(dataLen) {
			log.Printf("%s: offset-index row %d (key %d) is out of bounds (offset %d + size %d > data %d) — skipping row",
				name, i, entry.Key, entry.Offset, entry.Size, dataLen)
			continue
		}
		out = append(out, entry)
	}

	out = dropOverlappingRows(name, out)
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: offset index declares %d rows, none usable", name, count)
	}
	return out, nil
}

// dropOverlappingRows removes rows whose [offset, offset+size) range overlaps a
// row that already claimed those bytes. Overlap is only visible in offset order,
// so it's detected over a sorted view while out keeps its original index order.
func dropOverlappingRows(name string, out []IndexEntry) []IndexEntry {
	if len(out) < 2 {
		return out
	}

	ordered := make([]int, len(out))
	for i := range ordered {
		ordered[i] = i
	}
	sort.Slice(ordered, func(a, b int) bool {
		return out[ordered[a]].Offset < out[ordered[b]].Offset
	})

	drop := make(map[int]bool)
	prev := -1
	for _, i := range ordered {
		if prev >= 0 && out[i].Offset < out[prev].Offset+out[prev].Size {
			log.Printf("%s: offset-index key %d (offset %d, size %d) overlaps key %d (offset %d, size %d) — skipping row",
				name, out[i].Key, out[i].Offset, out[i].Size, out[prev].Key, out[prev].Offset, out[prev].Size)
			drop[i] = true
			continue
		}
		prev = i
	}
	if len(drop) == 0 {
		return out
	}

	kept := out[:0]
	for i, entry := range out {
		if !drop[i] {
			kept = append(kept, entry)
		}
	}
	return kept
}
