package tables

import (
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
	"github.com/idevelopthings/bdo-data-extractor/src/models"
)

// exploration.bss — the worldmap node list (the node-manager network: towns,
// gateways, farms, forests, mines, …). PABR with a UTF-16 Korean name string
// table. Byte-packed records: a fixed 117-byte head followed by SEVEN counted
// u32 lists, then a small footer table after the last record:
//
//	+0    u16 nodeKey       (== loc table 29 id, the localized node name)
//	+4    u8  flag (1)
//	+5    u8  nodeKind      (town/gate/farm/… enum; matches the worldmap icon)
//	+6    u16 linkedKey     (usually == nodeKey)
//	+10   u16 nameStrIdx    Korean node name (string table)
//	+23   u32 subKey        (waypoint-space key, written twice: also at +27)
//	+104  f32×3 position    world x/y/z
//	+117  7 × [u32 count][count × u32]   (unidentified id lists; usually
//	      list0 = 1 hash, list1 = a few small ids, rest empty)
//
//	footer: [u32 count][count × 6 bytes] (unidentified index table)
//
// Node→territory is NOT stored here (the game resolves it via
// waypoint→region); node CONNECTIONS are not client-side either. The build
// derives a territory from the nearest region in regioninfo.bss.
func DecodeExplorationNodes(data []byte) ([]model.WorldNode, error) {
	h, err := bss.OpenPABR(data)
	if err != nil {
		return nil, fmt.Errorf("exploration: %w", err)
	}
	rows, stPtr := h.Rows, h.StringTablePos
	strs := bss.ReadUTF16StringTable(data, stPtr)

	out := make([]model.WorldNode, 0, rows)
	o := 8
	for i := 0; i < rows; i++ {
		if o+121 > stPtr {
			return nil, fmt.Errorf("exploration: record %d truncated at %d", i, o)
		}
		key := int(bss.U16(data, o))
		nameIdx := bss.U16(data, o+10)
		if key == 0 || int(nameIdx) >= len(strs) {
			return nil, fmt.Errorf("exploration: record %d invalid (key=%d nameIdx=%d)", i, key, nameIdx)
		}
		node := model.WorldNode{
			BaseFor: models.NewBaseFor[model.WorldNode](uint32(key), "node"),
			Key:     key,
			Kind:    int(data[o+5]),
			Name:    strs[nameIdx], // replaced by the loc-29 join when available
			Position: [3]float64{
				bss.F32(data, o+104), bss.F32(data, o+108), bss.F32(data, o+112),
			},
		}
		// seven counted u32 lists determine the record length
		p := o + 117
		for l := 0; l < 7; l++ {
			c := int(bss.U32(data, p))
			if c < 0 || c > 300 || p+4+4*c > stPtr {
				return nil, fmt.Errorf("exploration: record %d list %d bad count %d (layout changed?)", i, l, c)
			}
			p += 4 + 4*c
		}
		out = append(out, node)
		o = p
	}
	// footer: [u32 count][count × 6 bytes]
	if o+4 > stPtr {
		return nil, fmt.Errorf("exploration: footer missing at %d", o)
	}
	foot := int(bss.U32(data, o))
	if o+4+foot*6 != stPtr {
		return nil, fmt.Errorf("exploration: %d records + footer(%d) consumed %d of %d record bytes",
			rows, foot, o+4+foot*6-8, stPtr-8)
	}
	return out, nil
}
