package tables

import (
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

// CrystalGroupRule is one transfusion-family limit from
// jewelgroupstaticstatus.bss.
type CrystalGroupRule struct {
	Key        uint16
	SourceName string
	Max        uint16
}

// DecodeCrystalGroupRules reads every fixed crystal-group row.
func DecodeCrystalGroupRules(data []byte) ([]CrystalGroupRule, error) {
	pabr, err := bss.OpenPABR(data)
	if err != nil {
		return nil, fmt.Errorf("jewelgroupstaticstatus: %w", err)
	}
	recordSize, fixed := pabr.RecordSize()
	if !fixed || recordSize != 8 {
		return nil, fmt.Errorf("jewelgroupstaticstatus: record size %d, want 8", recordSize)
	}
	strings := bss.ReadUTF16StringTable(data, pabr.StringTablePos)
	if len(strings) == 0 {
		return nil, fmt.Errorf("jewelgroupstaticstatus: empty string table")
	}
	out := make([]CrystalGroupRule, 0, pabr.Rows)
	seen := make(map[uint32]bool, pabr.Rows)
	for row := range pabr.Rows {
		c := bss.NewCursor(data, pabr.RecordsStart+row*recordSize, pabr.RecordsStart+(row+1)*recordSize)
		nameIndex := int(c.U32())
		key := uint16(c.U16())
		maxCount := uint16(c.U16())
		if !c.OK() || c.Remaining() != 0 {
			return nil, fmt.Errorf("jewelgroupstaticstatus: row %d is truncated", row)
		}
		if nameIndex < 0 || nameIndex >= len(strings) {
			return nil, fmt.Errorf("jewelgroupstaticstatus: group %d has string index %d, table has %d", key, nameIndex, len(strings))
		}
		if seen[uint32(key)] {
			return nil, fmt.Errorf("jewelgroupstaticstatus: duplicate group %d", key)
		}
		seen[uint32(key)] = true
		out = append(out, CrystalGroupRule{Key: key, SourceName: strings[nameIndex], Max: maxCount})
	}
	return out, nil
}

// CrystalSpecialSlotRule restricts a special preset slot to crystal groups.
type CrystalSpecialSlotRule struct {
	Slot          model.CrystalSpecialSlot
	AllowedGroups []uint16
}

// DecodeCrystalSpecialSlotRules reads the variable counted group lists in
// jewelspecialslotsgroupstaticstatus.bss.
func DecodeCrystalSpecialSlotRules(data []byte) ([]CrystalSpecialSlotRule, error) {
	pabr, err := bss.OpenPABR(data)
	if err != nil {
		return nil, fmt.Errorf("jewelspecialslotsgroupstaticstatus: %w", err)
	}
	c := bss.NewCursor(data, pabr.RecordsStart, pabr.StringTablePos)
	out := make([]CrystalSpecialSlotRule, 0, pabr.Rows)
	seen := make(map[model.CrystalSpecialSlot]bool, pabr.Rows)
	for row := range pabr.Rows {
		slot := model.CrystalSpecialSlot(c.U8())
		count := int(c.U32())
		if count <= 0 || count > 64 {
			return nil, fmt.Errorf("jewelspecialslotsgroupstaticstatus: row %d slot %d has bad group count %d", row, slot, count)
		}
		groups := make([]uint16, count)
		for i := range groups {
			groups[i] = uint16(c.U16())
		}
		if !c.OK() {
			return nil, fmt.Errorf("jewelspecialslotsgroupstaticstatus: row %d slot %d is truncated", row, slot)
		}
		if seen[slot] {
			return nil, fmt.Errorf("jewelspecialslotsgroupstaticstatus: duplicate slot %d", slot)
		}
		seen[slot] = true
		out = append(out, CrystalSpecialSlotRule{Slot: slot, AllowedGroups: groups})
	}
	if !c.OK() || c.Remaining() != 0 {
		return nil, fmt.Errorf("jewelspecialslotsgroupstaticstatus: records consume %d of %d bytes", c.Pos(), pabr.StringTablePos)
	}
	return out, nil
}
