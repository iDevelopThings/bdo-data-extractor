package tables

import (
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
	"github.com/idevelopthings/bdo-data-extractor/src/models"
)

// DecodeZones decodes all 105 Monster Zone Info records. The table is byte-
// packed (no field alignment), and every record ends in a 24-byte stat block:
//
//	[offenseAP f][defenseDP f][waypointKey u32][totalAP f][totalDP f][effLimit u32]
//
// Those blocks are found by a byte-by-byte scan (105 = the row count), and each
// one anchors a record: record i spans from the previous block's end to this
// one. Within a record the layout is, in order: a header (lapApplyPercent,
// huntingGroundKey, mainCategory, sub-categories, sortIndex), then length-
// prefixed sections — ecology creatures(u16), recurring quests(u32 group|index<<16),
// region quests(u32), loot item ids(u32), tags(u32 → dropuitaginfo keys), topography
// place ids(u16), titles(u32) — then the 3-float nav position, then the stat block.
// Record 0 uses a compact header variant. Validated: sortIndex == record index for all 105,
// and the section walk lands exactly on the stat block for 102/105 (the rest get
// stat + name only). isItem reports whether an id is a known item.
func DecodeZones(data []byte, isItem func(uint32) bool) ([]model.Zone, error) {
	pabr, err := bss.OpenPABR(data)
	if err != nil {
		return nil, err
	}
	stPtr := pabr.StringTablePos
	statF := func(o int) bool { f := bss.F32(data, o); return f >= 1 && f <= 3000 }

	// byte-scan the stat-block offsets (one per record), skipping past each match
	var stat []int
	for p := 8; p+24 <= stPtr; {
		if statF(p) && statF(p+4) && statF(p+12) && statF(p+16) {
			node, eff := bss.U32(data, p+8), bss.U32(data, p+20)
			if eff >= 1 && eff <= 6000 && (node == 0 || (node >= 50 && node <= 5000)) {
				stat = append(stat, p)
				p += 24
				continue
			}
		}
		p++
	}
	if len(stat) == 0 {
		return nil, fmt.Errorf("dropuihuntinggroundinfo: no zone stat blocks found")
	}

	zones := make([]model.Zone, 0, len(stat))
	for i, sOff := range stat {
		z := model.Zone{
			Node:           &model.NodeRef{Key: bss.U32(data, sOff+8)},
			SheetAP:        rnd(bss.F32(data, sOff)),
			SheetDP:        rnd(bss.F32(data, sOff+4)),
			TotalAP:        rnd(bss.F32(data, sOff+12)),
			TotalDP:        rnd(bss.F32(data, sOff+16)),
			EffectiveLimit: int(bss.U32(data, sOff+20)),
		}
		// nav position: 3 floats before the stat block (0,0,0 for node-based zones)
		if x, y, zc := bss.F32(data, sOff-12), bss.F32(data, sOff-8), bss.F32(data, sOff-4); x != 0 || y != 0 || zc != 0 {
			z.Node.Pos = []float64{round2(x), round2(y), round2(zc)}
		}

		// walk the record's sections from its anchored start up to the nav block
		start := 8
		if i > 0 {
			start = stat[i-1] + 24
		}
		c := bss.NewCursor(data, start, sOff-12)
		z.ApplyPercent = int(c.U32()) // lapApplyPercent
		z.Key = c.U32()               // huntingGroundKey
		z.BaseFor = models.NewBaseFor[model.Zone](z.Key)
		if i > 0 {
			c.U8()                                        // header pad
			z.MainCategory = &model.Category{ID: c.U32()} // mainCategoryKey
			for sc := int(c.U32()); sc > 0 && sc < 32 && c.OK(); sc-- {
				z.SubCategories = append(z.SubCategories, model.Category{ID: c.U32()})
			}
		} else {
			z.MainCategory = &model.Category{ID: c.U32()} // mainCategoryKey
			c.U32()                                       // headerA (record 0 compact variant)
		}
		c.U32() // sortIndex (== i)

		ecology := c.U16List(40)      // ecology creature knowledge (loc table-6 keys)
		recurring := questList(c, 30) // repeat quests
		sudden := questList(c, 30)    // region quests
		loot := c.U32List(60)         // drop items
		tags := c.U32List(40)         // dropuitaginfo keys (1..44)
		topography := c.U16List(40)   // place/topography knowledge (loc table-17 keys)
		titles := c.U32List(20)       // title ids

		// only trust the sections if the walk landed exactly on the nav block
		if c.OK() && c.Pos() == sOff-12 {
			z.Tags = tagRefs(tags)
			z.RecurringQuests = recurring
			z.RegionQuests = sudden
			z.Titles = refs(titles)
			z.Ecology = refs(ecology)
			z.Topography = refs(topography)
			var lootIDs []uint32
			for _, it := range loot {
				if isItem(it) {
					lootIDs = append(lootIDs, it)
				}
			}
			z.Loot = model.ItemRefList(lootIDs...)
		}
		zones = append(zones, z)
	}
	return zones, nil
}

// DecodeTags decodes dropuitaginfo.bss — a PABR table whose numeric section is
// 44 fixed 32-byte rows: [key u32][4 unknown u32][textureColor u32][fontColor u32].
// (String payload — the Korean labels — is ignored; labels come from loc.)
func DecodeTags(data []byte) ([]model.TagInfo, error) {
	pabr, err := bss.OpenPABR(data)
	if err != nil {
		return nil, err
	}
	if pabr.Rows <= 0 || pabr.Rows > 1000 {
		return nil, fmt.Errorf("dropuitaginfo: bad row count %d", pabr.Rows)
	}
	out := make([]model.TagInfo, 0, pabr.Rows)
	for i := 0; i < pabr.Rows; i++ {
		o := pabr.RecordsStart + i*32
		if o+32 > len(data) {
			return nil, fmt.Errorf("dropuitaginfo: truncated at row %d", i)
		}
		out = append(
			out, model.TagInfo{
				Key:       bss.U32(data, o),
				Color:     fmt.Sprintf("0x%08X", bss.U32(data, o+24)),
				FontColor: fmt.Sprintf("0x%08X", bss.U32(data, o+28)),
			},
		)
	}
	return out, nil
}

// questList reads a [u32 count][count×u32] block and formats each entry from its
// packed group | (index<<16) form into the public "group-index" quest id.
func questList(c *bss.Cursor, max int) []model.QuestRef {
	ids := c.U32List(max)
	out := make([]model.QuestRef, 0, len(ids))
	for _, v := range ids {
		out = append(out, model.QuestRef{ID: fmt.Sprintf("%d-%d", v&0xFFFF, v>>16)})
	}
	return out
}

// refs wraps a list of ids as unresolved Refs (the build fills in their names).
func refs(ids []uint32) []model.Ref {
	out := make([]model.Ref, 0, len(ids))
	for _, id := range ids {
		out = append(out, model.Ref{ID: id})
	}
	return out
}

// tagRefs wraps dropuitaginfo keys as TagInfos (the build fills label/desc/color).
func tagRefs(keys []uint32) []model.TagInfo {
	out := make([]model.TagInfo, 0, len(keys))
	for _, k := range keys {
		out = append(out, model.TagInfo{Key: k})
	}
	return out
}
