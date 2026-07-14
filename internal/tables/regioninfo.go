package tables

import (
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
	"github.com/idevelopthings/bdo-data-extractor/src/models"
)

// regioninfo.bss — the geographic region database: every map region's key, name,
// parent area, territory, world position and (for warehouse-bearing places) the
// storage/transport group.
//
// PABR with a UTF-16 Korean name string table. Records are byte-packed with a
// FIXED 389-byte skeleton plus one variable list, so consecutive records shift
// u32 alignment by one byte (389 ≡ 1 mod 4) — the reason earlier aligned scans
// saw only "smeared" values. Layout (offsets from record start):
//
//	+0    u16 regionKey       == loc table 17 id (English place name) and the
//	                             regionclientdata/region_info.xml region key
//	+2    u8  unk, +3 u8 unk
//	+4    u16 unk
//	+6    u8  regionType      1 = town/city (has connections), 2 = field, …
//	+7    u8  unk
//	+32   u32 const 19950     (record anchor A)
//	+90   u8  territoryIndex  == territoryinfo.bss index / loc table 12 id
//	+92   u16 nameStrIdx      own Korean name (string table)
//	+96   u16 capitalNameIdx  the TERRITORY CAPITAL's Korean name
//	+100  u16 capitalKey      the territory's capital/main region key — constant
//	                          across every record of a territory (Balenos → 5
//	                          Velia, Mediah → 202 Altinova; distinguishes the
//	                          two Edania territories)
//	+102  u16 unk related region key
//	+131  f32×3 position      world x/y/z
//	+149  u32 const 0x13524B01 (record anchor B; a version/locale marker — was
//	                          0x0B79B401 before a game patch bumped it)
//	+153  f32×5 unk params
//	+185  u32×6 unk ids       (~105k range; towns only)
//	+210  u32 warehouseGroupCount
//	+214  u16×count region keys of the warehouses in this place's storage/
//	      transport group (only the 58 warehouse-bearing places carry it; the
//	      groups are disjoint — mainland cluster, Valencia City↔Ancado,
//	      Morning Land villages, isolated islands listing only themselves)
//	then  u32 extraPositionCount + count×vec3f (worldmap mark positions for
//	      oversized zones — only the Great Desert of Valencia uses it)
//	then a fixed tail (map colors + constants)
//
// Record size = 389 + 2×warehouseGroupCount + 12×extraPositionCount. The decoder
// walks sequentially and re-validates both constant anchors on every record, so
// a post-patch layout change fails loudly instead of yielding garbage.
const (
	regionRecBase   = 389
	regionConstA    = 19950      // u32 @ +32
	regionConstB    = 0x13524B01 // u32 @ +149 — a version/locale marker ("KR"); bumped from 0x0B79B401 by a game patch
	offRegionType   = 6
	offTerritoryIdx = 90
	offNameIdx      = 92
	offCapitalKey   = 100
	offPosition     = 131
	offWhGroupCount = 210
	offWhGroupList  = 214
)

// DecodeRegionInfo reads regioninfo.bss into the geographic region list, plus
// each territory's capital region key (stored redundantly on every record; the
// decoder validates the redundancy and reports it once per territory). Korean
// names come from the table's own string table; English names are joined by the
// caller (loc table 17 via regionKey, loc table 12 via territoryIndex).
func DecodeRegionInfo(data []byte) ([]model.WorldRegion, map[int]int, error) {
	if len(data) < 16 || string(data[:4]) != "PABR" {
		return nil, nil, fmt.Errorf("regioninfo: not a PABR table")
	}
	rows := int(bss.U32(data, 4))
	stPtr := int(bss.U64(data, len(data)-8))
	if rows <= 0 || stPtr <= 8 || stPtr > len(data) {
		return nil, nil, fmt.Errorf("regioninfo: bad header (rows=%d stPtr=%d)", rows, stPtr)
	}
	strs := bss.ReadUTF16StringTable(data, stPtr)

	out := make([]model.WorldRegion, 0, rows)
	capitals := map[int]int{}
	o := 8
	for i := 0; i < rows; i++ {
		if o+regionRecBase > stPtr {
			return nil, nil, fmt.Errorf("regioninfo: record %d truncated at %d", i, o)
		}
		if bss.U32(data, o+32) != regionConstA || bss.U32(data, o+149) != regionConstB {
			return nil, nil, fmt.Errorf("regioninfo: record %d anchors missing at %d (layout changed?)", i, o)
		}
		nameIdx := bss.U16(data, o+offNameIdx)
		if int(nameIdx) >= len(strs) {
			return nil, nil, fmt.Errorf("regioninfo: record %d name index out of range", i)
		}
		terr := int(data[o+offTerritoryIdx])
		capital := int(bss.U16(data, o+offCapitalKey))
		if prev, seen := capitals[terr]; seen && prev != capital {
			return nil, nil, fmt.Errorf("regioninfo: territory %d has conflicting capitals %d/%d (layout changed?)", terr, prev, capital)
		}
		capitals[terr] = capital
		count := int(bss.U32(data, o+offWhGroupCount))
		if count < 0 || count > 512 || o+regionRecBase+2*count > stPtr {
			return nil, nil, fmt.Errorf("regioninfo: record %d bad warehouse-group count %d", i, count)
		}
		var group []int
		for k := 0; k < count; k++ {
			group = append(group, int(bss.U16(data, o+offWhGroupList+2*k)))
		}
		posOff := o + offWhGroupList + 2*count
		posCount := int(bss.U32(data, posOff))
		if posCount < 0 || posCount > 16 || posOff+4+12*posCount > stPtr {
			return nil, nil, fmt.Errorf("regioninfo: record %d bad extra-position count %d", i, posCount)
		}
		var extra [][3]float64
		for k := 0; k < posCount; k++ {
			p := posOff + 4 + 12*k
			extra = append(extra, [3]float64{bss.F32(data, p), bss.F32(data, p+4), bss.F32(data, p+8)})
		}
		key := uint32(bss.U16(data, o))

		out = append(out, model.WorldRegion{
			BaseFor:   models.NewBaseFor[model.WorldRegion](key, "region"),
			Key:       int(key),
			Type:      int(data[o+offRegionType]),
			Territory: model.TerritoryRef(terr),
			// embedded Korean name; the build's loc join replaces it
			Name:           strs[nameIdx],
			Position:       [3]float64{bss.F32(data, o+offPosition), bss.F32(data, o+offPosition+4), bss.F32(data, o+offPosition+8)},
			ExtraPositions: extra,
			WarehouseGroup: group,
		})
		o += regionRecBase + 2*count + 12*posCount
	}
	if o != stPtr {
		return nil, nil, fmt.Errorf("regioninfo: %d records consumed %d of %d record bytes", rows, o-8, stPtr-8)
	}
	return out, capitals, nil
}
