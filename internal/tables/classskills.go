package tables

import (
	"fmt"
	"sort"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

const maxSkillGroupRanks = 64

// SkillGroup is one rank chain from skillgroup.bss. Rank zero is represented
// by the leading zero skill key used by the client for an unlearned group.
type SkillGroup struct {
	Key       uint16
	SkillKeys []uint32
}

// DecodeSkillGroups reads the complete skill-group rank map.
func DecodeSkillGroups(data []byte) (map[uint16]SkillGroup, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("skillgroup: truncated table")
	}
	count := int(bss.U32(data, 0))
	if count <= 0 {
		return nil, fmt.Errorf("skillgroup: bad row count %d", count)
	}
	c := bss.NewCursor(data, 4, len(data))
	out := make(map[uint16]SkillGroup, count)
	for row := range count {
		key := uint16(c.U16())
		skillKeys := c.U32List(maxSkillGroupRanks)
		if !c.OK() || len(skillKeys) == 0 {
			return nil, fmt.Errorf("skillgroup: row %d group %d has an invalid rank list", row, key)
		}
		if skillKeys[0] != 0 {
			return nil, fmt.Errorf("skillgroup: group %d rank-zero key is %#x", key, skillKeys[0])
		}
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("skillgroup: duplicate group %d", key)
		}
		out[key] = SkillGroup{Key: key, SkillKeys: skillKeys}
	}
	if !c.OK() || c.Remaining() != 0 {
		return nil, fmt.Errorf("skillgroup: records consume %d of %d bytes", c.Pos(), len(data))
	}
	return out, nil
}

// SkillTreeKind identifies which client skill panel owns a grid.
type SkillTreeKind string

const (
	SkillTreeCombat    SkillTreeKind = "combat"
	SkillTreeAwakening SkillTreeKind = "awakening"
)

// SkillTreeCell preserves one cell from a class skill grid. Types are the
// client's line/blank/group drawing types; type 2 identifies a skill group.
type SkillTreeCell struct {
	X         int
	Y         int
	Types     []byte
	Group     uint16
	Unknown16 byte
	SubGroup  byte
}

// HasSkillGroup reports whether the cell contains the client SkillGroup type.
func (c SkillTreeCell) HasSkillGroup() bool {
	for _, cellType := range c.Types {
		if cellType == 2 {
			return true
		}
	}
	return false
}

// SkillTree is one class row from a UI skill-group table.
type SkillTree struct {
	ClassType model.CharacterClassType
	Kind      SkillTreeKind
	Width     int
	Height    int
	Cells     []SkillTreeCell
}

// SkillTreeTable contains the class grids and the still-unidentified subgroup
// directory stored between the grids and the table's string keys.
type SkillTreeTable struct {
	Trees              []SkillTree
	SubGroupStringKeys []string
	UnknownFooter      []byte
}

// DecodeSkillTrees reads ui_skillgroup_combat.bss or
// ui_skillgroup_awakening.bss and consumes every variable-size cell.
func DecodeSkillTrees(data []byte, kind SkillTreeKind) (SkillTreeTable, error) {
	pabr, err := bss.OpenPABR(data)
	if err != nil {
		return SkillTreeTable{}, fmt.Errorf("ui skillgroup %s: %w", kind, err)
	}
	c := bss.NewCursor(data, pabr.RecordsStart, pabr.StringTablePos)
	out := make([]SkillTree, 0, pabr.Rows)
	seenClasses := make(map[model.CharacterClassType]bool, pabr.Rows)
	for row := range pabr.Rows {
		classType := model.CharacterClassType(c.U8())
		width, height := int(c.U32()), int(c.U32())
		if width <= 0 || width > 64 || height <= 0 || height > 512 {
			return SkillTreeTable{}, fmt.Errorf("ui skillgroup %s: row %d class %d has bad dimensions %dx%d", kind, row, classType, width, height)
		}
		if seenClasses[classType] {
			return SkillTreeTable{}, fmt.Errorf("ui skillgroup %s: duplicate class %d", kind, classType)
		}
		seenClasses[classType] = true
		cells := make([]SkillTreeCell, 0, width*height)
		for cellIndex := range width * height {
			typeCount := int(c.U32())
			if typeCount <= 0 || typeCount > 32 {
				return SkillTreeTable{}, fmt.Errorf("ui skillgroup %s: class %d cell %d has bad type count %d", kind, classType, cellIndex, typeCount)
			}
			types := c.U8N(typeCount)
			packed := c.U32()
			if !c.OK() {
				return SkillTreeTable{}, fmt.Errorf("ui skillgroup %s: class %d cell %d is truncated", kind, classType, cellIndex)
			}
			cells = append(cells, SkillTreeCell{
				X:         cellIndex % width,
				Y:         cellIndex / width,
				Types:     types,
				Group:     uint16(packed),
				Unknown16: byte(packed >> 16),
				SubGroup:  byte(packed >> 24),
			})
		}
		out = append(out, SkillTree{ClassType: classType, Kind: kind, Width: width, Height: height, Cells: cells})
	}
	unknownFooter := append([]byte(nil), c.Bytes(c.Remaining())...)
	if !c.OK() || c.Remaining() != 0 {
		return SkillTreeTable{}, fmt.Errorf("ui skillgroup %s: footer is truncated", kind)
	}
	return SkillTreeTable{
		Trees:              out,
		SubGroupStringKeys: bss.ReadStringTable(data, pabr.StringTablePos),
		UnknownFooter:      unknownFooter,
	}, nil
}

// SkillTypeHeader is the identity prefix of a skilltype.dbss record. The
// remaining record is the skill's large action/presentation configuration.
type SkillTypeHeader struct {
	SkillKey        uint32
	SourceName      string
	SourceGroupName string
	Kind            model.SkillKind
}

// DecodeSkillTypeHeaders reads each skill's identity prefix and validates the
// offset-index tiling. The variable action configuration is consumed as an
// opaque tail because it has no bearing on skill identity or passive effects.
func DecodeSkillTypeHeaders(offsetData, data []byte) (map[uint32]SkillTypeHeader, error) {
	entries, err := bss.ParseOffsetIndex(offsetData, len(data))
	if err != nil {
		return nil, fmt.Errorf("skilltype: %w", err)
	}
	ordered := append([]bss.IndexEntry(nil), entries...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Offset < ordered[j].Offset
	})
	if len(ordered) == 0 {
		return nil, fmt.Errorf("skilltype: empty index")
	}
	if len(data) < 8 || int(bss.U32(data, 0)) != len(entries) {
		return nil, fmt.Errorf("skilltype: data count does not match %d index rows", len(entries))
	}
	if ordered[0].Offset != 8 || bss.U32(data, 4) != ordered[0].Key {
		return nil, fmt.Errorf("skilltype: first record framing does not match key %#x", ordered[0].Key)
	}
	for i, entry := range ordered {
		end := entry.Offset + entry.Size
		if i+1 == len(ordered) {
			if end != uint32(len(data)) {
				return nil, fmt.Errorf("skilltype: records end at %d, data ends at %d", end, len(data))
			}
			break
		}
		next := ordered[i+1]
		if end+4 != next.Offset || bss.U32(data, int(end)) != next.Key {
			return nil, fmt.Errorf("skilltype: record %#x ends at %d without the next key %#x framing", entry.Key, end, next.Key)
		}
	}

	out := make(map[uint32]SkillTypeHeader, len(entries))
	for rec, err := range bss.RecordsFromEntries(entries, data) {
		if err != nil {
			return nil, fmt.Errorf("skilltype: key %#x: %w", rec.Entry.Key, err)
		}
		c := bss.NewCursor(rec.Data, 0, len(rec.Data))
		header := SkillTypeHeader{
			SkillKey:        c.U32(),
			SourceName:      c.UTF16(),
			SourceGroupName: c.UTF16(),
			Kind:            model.SkillKind(c.U32()),
		}
		_ = c.Bytes(c.Remaining())
		if err := bss.RequireExhausted(c); err != nil {
			return nil, fmt.Errorf("skilltype: key %#x: %w", rec.Entry.Key, err)
		}
		if header.SkillKey != rec.Entry.Key {
			return nil, fmt.Errorf("skilltype: index key %#x resolves to %#x", rec.Entry.Key, header.SkillKey)
		}
		if _, exists := out[rec.Entry.Key]; exists {
			return nil, fmt.Errorf("skilltype: duplicate key %#x", rec.Entry.Key)
		}
		out[rec.Entry.Key] = header
	}
	return out, nil
}
