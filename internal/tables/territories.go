package tables

import (
	"fmt"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
	"github.com/idevelopthings/bdo-data-extractor/src/models"
)

// DecodeTerritories reads territoryinfo.bss: the 14 world territories in game
// order (record index == loc table 12 id). PABR container with a UTF-16 string
// table; the record region tiles exactly with this byte-packed layout:
//
//	u16 index        — sequential 0..N-1
//	u8  primary      — 1 = the nation's direct/primary territory (직할령, or the
//	                   sole territory of single-territory nations)
//	u8  autonomous   — 1 = autonomous territory (자치령: Balenos, Serendia)
//	[3]vec3 f32      — worldmap territory-mark positions (zeroed slots unused)
//	u32 nationKey    — hash shared by all territories of the same nation
//	                   (Balenos/Serendia/Calpheon share the Republic of Calpheon
//	                   key; the two Edania rows share Alyaelli's)
//	u32 nationStrIdx — Korean nation name (string table)
//	u32 nameStrIdx   — Korean territory name
//	u32 iconLargeIdx — large worldmap-mark .dds path
//	u32 iconSmallIdx — small worldmap-mark .dds path
//	u32 const 2
//	u32 0
//	u32 crownItemId  — territory-conquest crown item (loc t0: 23381 "Silver Mane
//	                   Horse Crown" = Balenos …; frozen at Valencia's 23385 for
//	                   territories without a castle siege)
//	u32 armorItemId  — conquest armor item (23386 "Silver Mane's Armor" …)
//	u32 hasExtra     — 0/1: whether the optional key below is present
//	u32 0
//	[u32 extraKey]   — only when hasExtra == 1 (Serendia 114, Mediah 312;
//	                   not a region/node/loc key — meaning unidentified)
//	u32 0
//
// Every structural invariant is checked; a short/reordered record fails loudly
// rather than yielding garbage after a game patch.
func DecodeTerritories(data []byte) ([]model.Territory, error) {
	h, err := bss.OpenPABR(data)
	if err != nil {
		return nil, fmt.Errorf("territoryinfo: %w", err)
	}
	rows, stPtr := h.Rows, h.StringTablePos
	strs := bss.ReadUTF16StringTable(data, stPtr)
	str := func(i uint32) (string, error) {
		if int(i) >= len(strs) {
			return "", fmt.Errorf("territoryinfo: string index %d out of range (%d strings)", i, len(strs))
		}
		return strs[i], nil
	}

	c := bss.NewCursor(data, 8, stPtr)
	out := make([]model.Territory, 0, rows)
	for i := 0; i < rows; i++ {
		idx := int(c.U16())
		primary := c.U8()
		autonomous := c.U8()
		var positions [][3]float64
		for p := 0; p < 3; p++ {
			v := [3]float64{c.F32(), c.F32(), c.F32()}
			if v != ([3]float64{}) {
				positions = append(positions, v)
			}
		}
		nationKey := c.U32()
		nationKR, err := str(c.U32())
		if err != nil {
			return nil, err
		}
		nameKR, err := str(c.U32())
		if err != nil {
			return nil, err
		}
		iconLarge, err := str(c.U32())
		if err != nil {
			return nil, err
		}
		iconSmall, err := str(c.U32())
		if err != nil {
			return nil, err
		}
		const2 := c.U32()
		zero1 := c.U32()
		crownItem := c.U32()
		armorItem := c.U32()
		hasExtra := c.U32()
		zero2 := c.U32()
		var extraKey uint32
		if hasExtra == 1 {
			extraKey = c.U32()
		}
		zero3 := c.U32()

		if !c.OK() {
			return nil, fmt.Errorf("territoryinfo: record %d truncated", i)
		}
		if idx != i {
			return nil, fmt.Errorf("territoryinfo: record %d has index %d (layout changed?)", i, idx)
		}
		if const2 != 2 || zero1 != 0 || zero2 != 0 || zero3 != 0 || hasExtra > 1 {
			return nil, fmt.Errorf("territoryinfo: record %d const fields moved (c2=%d z=%d,%d,%d has=%d)",
				i, const2, zero1, zero2, zero3, hasExtra)
		}

		out = append(out, model.Territory{
			BaseFor:     models.NewBaseFor[model.Territory](uint32(idx), "territory"),
			Index:       idx,
			Name:        nameKR, // replaced by the loc join when available
			Nation:      nationKR,
			NationKey:   nationKey,
			Primary:     primary == 1,
			Autonomous:  autonomous == 1,
			Positions:   positions,
			IconLarge:   iconLarge, // raw .dds path; the build maps it to icons/territories/
			IconSmall:   iconSmall,
			CrownItemID: model.ItemRef(crownItem),
			ArmorItemID: model.ItemRef(armorItem),
			ExtraKey:    extraKey,
		})
	}
	if c.Pos() != stPtr {
		return nil, fmt.Errorf("territoryinfo: %d records consumed %d of %d record bytes", rows, c.Pos()-8, stPtr-8)
	}
	return out, nil
}

// TerritoryIconFile maps a territory-mark texture path from territoryinfo.bss
// (e.g. "Renewal/ETC/WordMap/territorymark_valenos_large.dds") to the PNG file
// name the icons command writes under icons/territories/.
func TerritoryIconFile(ddsPath string) string {
	p := strings.ReplaceAll(strings.ToLower(ddsPath), "\\", "/")
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		p = p[i+1:]
	}
	return strings.TrimSuffix(p, ".dds") + ".png"
}

// TerritoryIconArchivePath maps the same stored texture path to its full PAZ
// archive path (the stored paths are relative to ui_texture/).
func TerritoryIconArchivePath(ddsPath string) string {
	return "ui_texture/" + strings.ToLower(strings.ReplaceAll(ddsPath, "\\", "/"))
}

// TerritoryIconFiles resolves each distinct raw territory-mark path to its output
// PNG name. A basename is kept as-is when it's unique; when two territories carry a
// mark with the same filename from different folders (Valencia and The Great Ocean
// both use a "valencia" mark, stored in different UI folders with different art), the
// shared name is disambiguated by its parent folder so both survive as distinct
// files. Both the build (world.json) and the icon extractor call this so the paths
// they record and write agree.
func TerritoryIconFiles(raws []string) map[string]string {
	base := map[string]int{}
	uniq := map[string]bool{}
	for _, r := range raws {
		if r == "" || uniq[r] {
			continue
		}
		uniq[r] = true
		base[TerritoryIconFile(r)]++
	}
	out := make(map[string]string, len(uniq))
	for r := range uniq {
		name := TerritoryIconFile(r)
		if base[name] > 1 {
			name = parentFolder(r) + "_" + name
		}
		out[r] = name
	}
	return out
}

// parentFolder returns the immediate parent directory segment of a territory-mark
// path (e.g. ".../wordmap/mark.dds" -> "wordmap"), lowercased with backslashes
// normalized, or "" if there is none.
func parentFolder(ddsPath string) string {
	p := strings.ReplaceAll(strings.ToLower(ddsPath), "\\", "/")
	i := strings.LastIndexByte(p, '/')
	if i < 0 {
		return ""
	}
	j := strings.LastIndexByte(p[:i], '/')
	return p[j+1 : i]
}
