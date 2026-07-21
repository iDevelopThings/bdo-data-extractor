package tables

import (
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/src/utils"
)

const (
	itemImprovementDiscReform = 5
	itemImprovementSizeReform = 41
	itemImprovementSizeAlt    = 25 // disc=1; item-only rule with no source/base relation
)

// ItemImprovement is one gear reform row from itemimprovement.dbss (disc=5):
// result item produced from up to four base item slots (unused slots repeat the primary base).
type ItemImprovement struct {
	Key    uint32
	Result uint32
	Bases  []uint32 // unique non-zero base item ids, primary first
	Flag   uint32   // unidentified N×256 value
}

// DecodeItemImprovements reads reform rows from the offset-indexed
// itemimprovement.dbss table. Item-only 25-byte rows are validated and counted,
// but are not returned because they contain no source/base item relation.
func DecodeItemImprovements(offsetRaw, data []byte) (rows []ItemImprovement, altCount int, err error) {
	entries, err := bss.ParseOffsetIndex(offsetRaw, len(data))
	if err != nil {
		return nil, 0, fmt.Errorf("itemimprovement offset: %w", err)
	}
	if hdr := bss.U32(data, 0); int(hdr) != len(entries) {
		return nil, 0, fmt.Errorf("itemimprovement: data count %d != index count %d", hdr, len(entries))
	}
	rows = make([]ItemImprovement, 0, len(entries))
	seenKey := make(map[uint32]bool, len(entries))
	for _, entry := range entries {
		rec, ok := entry.Slice(data)
		if !ok {
			return nil, 0, fmt.Errorf("itemimprovement key %d: invalid indexed slice", entry.Key)
		}
		switch entry.Size {
		case itemImprovementSizeAlt:
			c := bss.NewCursor(rec, 0, len(rec))
			disc := c.U32()
			pad := c.U32()
			item := c.U32()
			flag := c.U32()
			if !c.Zero(9) || !c.OK() || c.Remaining() != 0 {
				return nil, 0, fmt.Errorf("itemimprovement key %d: invalid item-only row", entry.Key)
			}
			if disc != 1 || pad != 0 || item == 0 || flag == 0 || flag%256 != 0 {
				return nil, 0, fmt.Errorf("itemimprovement key %d: invalid item-only fields %d/%d/%d/%d", entry.Key, disc, pad, item, flag)
			}
			altCount++
			continue
		case itemImprovementSizeReform:
			// ok
		default:
			return nil, 0, fmt.Errorf("itemimprovement key %d: record size %d, want %d or %d", entry.Key, entry.Size, itemImprovementSizeReform, itemImprovementSizeAlt)
		}
		if seenKey[entry.Key] {
			return nil, 0, fmt.Errorf("itemimprovement: duplicate key %d", entry.Key)
		}
		seenKey[entry.Key] = true

		c := bss.NewCursor(rec, 0, len(rec))
		disc := c.U32()
		pad := c.U32()
		result := c.U32()
		basesRaw := [4]uint32{c.U32(), c.U32(), c.U32(), c.U32()}
		flag := c.U32()
		if n := c.Fill(0); n != 9 {
			return nil, 0, fmt.Errorf("itemimprovement key %d: trailing pad %d bytes, want 9 zeros", entry.Key, n)
		}
		if !c.OK() || c.Remaining() != 0 {
			return nil, 0, fmt.Errorf("itemimprovement key %d: truncated or leftover", entry.Key)
		}
		if disc != itemImprovementDiscReform {
			return nil, 0, fmt.Errorf("itemimprovement key %d: disc %d, want %d", entry.Key, disc, itemImprovementDiscReform)
		}
		if pad != 0 {
			return nil, 0, fmt.Errorf("itemimprovement key %d: pad %d, want 0", entry.Key, pad)
		}
		if result == 0 {
			return nil, 0, fmt.Errorf("itemimprovement key %d: empty result", entry.Key)
		}
		bases := make([]uint32, 0, len(basesRaw))
		for _, id := range basesRaw {
			if id != 0 {
				bases = utils.AppendUnique(bases, id)
			}
		}
		if len(bases) == 0 {
			return nil, 0, fmt.Errorf("itemimprovement key %d: no base items", entry.Key)
		}
		rows = append(rows, ItemImprovement{
			Key:    entry.Key,
			Result: result,
			Bases:  bases,
			Flag:   flag,
		})
	}
	return rows, altCount, nil
}
