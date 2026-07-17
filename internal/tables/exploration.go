package tables

import (
	"fmt"
	"log"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
	"github.com/idevelopthings/bdo-data-extractor/src/models"
)

// DecodeExplorationNodes
// exploration.bss — the worldmap node list (the node-manager network: towns,
// gateways, farms, forests, mines, …). PABR with a UTF-16 Korean name string
// table. Byte-packed records: a fixed 117-byte head followed by SEVEN counted
// u32 lists, then a global footer table after the last record:
//
//	+0    u16 key           (== loc table 29 id, the localized node name)
//	+4    u8  enabled       (1 for active nodes; 0 on seven unused/unlocalized records)
//	+5    u8  kind          (town/gate/farm/… enum; matches the worldmap icon)
//	+6    u16 linkedKey     (a second node reference, == key in every current record)
//	+10   u16 nameStrIdx    Korean node name (string table)
//	+14   u8  const 1
//	+16   u8  1 = standard network node, 0 = special location (town/bank/sea/district/battlefield)
//	+17   u8  unknown flag (tracks main except for Ossuary and Velia Beach)
//	+18   u8  == main (a copy of +116)
//	+19   u8  special-content index (islands/grind/castle/battlefield; 63 nodes)
//	+20   u8  special-content category (1 island, 2 coastal, 5 inland/desert grind, 6 battlefield)
//	+21   u8  grind-zone marker (Marni/Elvia set; unique index; 25 nodes)
//	+22   u8  endgame grind area (AP-map tier: 2, or 3 = Star's End; 12 nodes)
//	+23   u32 subKey        (waypoint-space key)
//	+27   u32 subKey2       (== subKey for 997/1037 nodes, 0 for the other 40)
//	+31   f32 radius
//	+35   f32 radius²       (cached square of radius)
//	+39   u32              (small internal value, unidentified)
//	+43   u16 managerFamilyId (character id repeated on every affiliated node; 0 when absent)
//	+45   u16 representativeId (town ruler/representative character id; 0 when absent)
//	+47   u32 0x20000 | id  (bit17 const | per-main-node enumeration id; 0 on subs)
//	+51   u32 areaId<<16    (worldmap area/sector id; 44 areas; low 16 bits zero)
//	+55   zero             (+55..+90)
//	+94   u8  contribution  (contribution-point cost: 0 town, 1-3)
//	+95   zero             (+95..+103)
//	+104  f32×3 position    exploration/label anchor (= nodeStaticStatus:getPosition();
//	                        NOT a reliable parent relation and NOT the worldmap/minimap
//	                        pin). Map pins + connection
//	                        edges live in mapdata_realexplore2.bwp.
//	+116  u8  main flag     (0 = main node, 1 = sub; == !bdolytics.main, 999/999)
//	+117  7 × [u32 count][count × u32]
//	      list0 = one grouping hash (33 distinct values shared across nodes);
//	      lists 1-5 = KNOWLEDGE entry keys the game ties to this node (its NPCs,
//	      creatures and topography — every value is a knowledge.json key);
//	      list6 = empty.
//
//	footer: [u32 count][count × 6 bytes]  a (u16 key, u16 nodeKey, u16 0)
//	        lookup whose second column is always a node key — an index, not edges.
//
// The build joins +43 to characterfunction.dbss: its ordered family list selects
// the one node that retains Manager, while affiliates receive ManagerNode.
//
// Children are NOT stored in this table. The build derives them from the
// mapdata_realexplore2.bwp graph: every non-main neighbor of a main node is its child.
//
// Node→territory is NOT stored here (the build derives it from the nearest
// region in regioninfo.bss). The navigable/map position and node graph live in
// mapdata_realexplore2.bwp; build overlays those onto Position and keeps +104 as
// ExplorationPosition when they differ.
func DecodeExplorationNodes(data []byte) ([]model.WorldNode, error) {
	h, err := bss.OpenPABR(data)
	if err != nil {
		return nil, fmt.Errorf("exploration: %w", err)
	}
	strs := bss.ReadUTF16StringTable(data, h.StringTablePos)
	c := bss.NewCursor(data, h.RecordsStart, h.StringTablePos)

	out := make([]model.WorldNode, 0, h.Rows)
	// The regions below read as all-zero across every current record; we drop them rather than
	// store them. That's an observation, not a guarantee — track it so a patch that starts
	// writing data there (a newly-added field) surfaces instead of being silently discarded.
	zeroDrift := false
	for i := 0; i < h.Rows; i++ {
		key := int(c.U16())  // +0
		unk2 := int(c.U16()) // +2
		enabled := c.Bool()  // +4: active/usable record
		k := c.U8()
		kind := model.WorldNodeKind(k)               // +5
		linkedKey := int(c.U16())                    // +6
		unk8 := int(c.U16())                         // +8
		nameIdx := int(c.U16())                      // +10
		reservedOK := c.Zero(2)                      // +12..+13 (zero)
		const14 := c.U8()                            // +14 (const 1)
		reservedOK = c.Zero(1) && reservedOK         // +15 (zero)
		special := c.U8() == 0                       // +16: 0 = special location, 1 = network node
		unknown17 := c.Bool()                        // +17 (closely tracks main, with three exceptions)
		mainCopy := c.Bool()                         // +18 (== main)
		zoneIndex := int(c.U8())                     // +19 special-content index
		zoneCategory := int(c.U8())                  // +20 special-content category
		grindZone := int(c.U8())                     // +21 grind-zone marker (Marni/Elvia)
		grindTier := int(c.U8())                     // +22 endgame grind-area AP tier
		subKey := int(c.U32())                       // +23
		subKey2 := int(c.U32())                      // +27 (a second key, often 0)
		radius := c.F32()                            // +31
		c.F32()                                      // +35 (radius², derived)
		unk39 := int(c.U32())                        // +39
		managerFamilyID := int(c.U16())              // +43 repeated manager-family character id
		representativeID := int(c.U16())             // +45 town ruler/representative character id
		nodeIndex := int(c.U32()) & 0x1FFFF          // +47 enumeration id (drop the const 0x20000 flag)
		areaID := int(c.U32()) >> 16                 // +51 area/sector id (stored as areaId<<16)
		reservedOK = c.Zero(36) && reservedOK        // +55..+90 (zero)
		reservedOK = c.Zero(3) && reservedOK         // +91..+93 (zero)
		contribution := c.U8()                       // +94 contribution-point cost (0 town, 1-3)
		reservedOK = c.Zero(9) && reservedOK         // +95..+103 (zero)
		pos := [3]float64{c.F32(), c.F32(), c.F32()} // +104/+108/+112
		main := c.U8() == 0                          // +116: 0 = main node, 1 = sub; lists start at +117
		if const14 != 1 {
			return nil, fmt.Errorf("exploration: node %d has +14 constant %d, want 1", key, const14)
		}
		if mainCopy != main {
			return nil, fmt.Errorf("exploration: node %d has +18 main copy %v, want %v", key, mainCopy, main)
		}

		// seven counted u32 lists: list0 = grouping hash, lists 1-5 = knowledge keys.
		var groupHash, knowledge []int
		seen := map[uint32]bool{}
		for l := 0; l < 7; l++ {
			list := c.U32List(300)
			if l == 0 {
				for _, v := range list {
					groupHash = append(groupHash, int(v))
				}
				continue
			}
			for _, v := range list {
				if !seen[v] {
					seen[v] = true
					knowledge = append(knowledge, int(v))
				}
			}
		}
		if !c.OK() {
			return nil, fmt.Errorf("exploration: record %d walk failed at %d (layout changed?)", i, c.Pos())
		}
		if key == 0 || nameIdx >= len(strs) {
			return nil, fmt.Errorf("exploration: record %d invalid (key=%d nameIdx=%d)", i, key, nameIdx)
		}
		// ranges that read as all-zero across every current record; warn if that ever breaks.
		if !reservedOK {
			zeroDrift = true
		}

		out = append(out, model.WorldNode{
			BaseFor:            models.NewBaseFor[model.WorldNode](uint32(key), "node"),
			Key:                key,
			Kind:               kind,
			Name:               strs[nameIdx], // replaced by the loc-29 join when available
			Position:           pos,
			LinkedKey:          linkedKey,
			SubKey:             subKey,
			Knowledge:          model.KnowledgeEntryRefList(knowledge...),
			Main:               main,
			Contribution:       contribution,
			Radius:             radius,
			Manager:            model.NPCRef(uint32(managerFamilyID)),
			TownRepresentative: model.NPCRef(uint32(representativeID)),
			Enabled:            enabled,
			Unknown17:          unknown17,
			SubKey2:            subKey2,
			GroupHash:          groupHash,
			Unknown2:           unk2,
			Unknown8:           unk8,
			Special:            special,
			ZoneIndex:          zoneIndex,
			ZoneCategory:       zoneCategory,
			GrindZone:          grindZone,
			GrindTier:          grindTier,
			Unknown39:          unk39,
			NodeIndex:          nodeIndex,
			AreaID:             areaID,
		})
	}

	// footer: [u32 count][count × 6 bytes], a (u16, u16 nodeKey, u16) lookup; must tile exactly
	// up to the string table.
	foot := int(c.U32())
	footer := make([][3]uint32, 0, foot)
	for i := 0; i < foot; i++ {
		footer = append(footer, [3]uint32{c.U16(), c.U16(), c.U16()})
	}
	if !c.OK() || c.Pos() != h.StringTablePos {
		return nil, fmt.Errorf("exploration: footer(%d) ended at %d, want %d", foot, c.Pos(), h.StringTablePos)
	}

	if zeroDrift {
		log.Printf("exploration: WARNING a byte range assumed empty is now nonzero — a new field may have appeared; re-check the record layout")
	}

	return out, nil
}
