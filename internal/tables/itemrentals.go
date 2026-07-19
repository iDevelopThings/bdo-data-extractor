package tables

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
)

const buyItemByPointAction = "buyItemByPoint"

var buyItemByPointUTF16 = bss.EncodeUTF16(buyItemByPointAction)

// ItemRentalRow is one contribution-point rental action from an NPC dialogue.
type ItemRentalRow struct {
	CharacterKey      uint32
	DialogIndex       uint16
	ItemKey           uint32
	ItemSubKey        uint32
	Count             uint32
	PointType         uint32
	PointCost         uint32
	ConditionDSL      string
	SourceName        string
	SourceDescription string
	Unknown0          uint16
}

// DecodeItemRentals reads buyItemByPoint actions from detail_dialog.dbss. The
// paired index key packs the character key in its low word and dialog variant
// in its high word.
func DecodeItemRentals(offsetData, data []byte) ([]ItemRentalRow, error) {
	rows := make([]ItemRentalRow, 0)
	seen := make(map[ItemRentalRow]bool)
	for rec, err := range bss.IndexedRecords(offsetData, data) {
		if err != nil {
			return nil, fmt.Errorf("detail dialog index: %w", err)
		}
		decoded, err := decodeItemRentalRecord(rec.Data)
		if err != nil {
			return nil, fmt.Errorf("detail dialog %08x: %w", rec.Entry.Key, err)
		}
		for _, row := range decoded {
			row.CharacterKey = uint32(uint16(rec.Entry.Key))
			row.DialogIndex = uint16(rec.Entry.Key >> 16)
			if seen[row] {
				continue
			}
			seen[row] = true
			rows = append(rows, row)
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ItemKey != rows[j].ItemKey {
			return rows[i].ItemKey < rows[j].ItemKey
		}
		if rows[i].CharacterKey != rows[j].CharacterKey {
			return rows[i].CharacterKey < rows[j].CharacterKey
		}
		if rows[i].DialogIndex != rows[j].DialogIndex {
			return rows[i].DialogIndex < rows[j].DialogIndex
		}
		return rows[i].ConditionDSL < rows[j].ConditionDSL
	})

	return rows, nil
}

func decodeItemRentalRecord(record []byte) ([]ItemRentalRow, error) {
	if !bytes.Contains(record, buyItemByPointUTF16) {
		return nil, nil
	}
	startsByEnd := bss.IndexInlineUTF16ByEnd(record)
	rows := make([]ItemRentalRow, 0)
	for from := 0; from < len(record); {
		rel := bytes.Index(record[from:], buyItemByPointUTF16)
		if rel < 0 {
			break
		}
		charsOffset := from + rel
		from = charsOffset + len(buyItemByPointUTF16)
		actionOffset := charsOffset - 8
		if actionOffset < 0 {
			return nil, fmt.Errorf("rental action at offset %d has no string prefix", charsOffset)
		}
		candidates := make(map[ItemRentalRow]bool)
		for _, descriptionOffset := range startsByEnd[actionOffset] {
			labelEnd := descriptionOffset - 4
			if labelEnd < 0 {
				continue
			}
			for _, labelOffset := range startsByEnd[labelEnd] {
				conditionOffsets := append([]int(nil), startsByEnd[labelOffset]...)
				if labelOffset >= 8 && bss.U64(record, labelOffset-8) == 0 {
					conditionOffsets = append(conditionOffsets, labelOffset-8)
				}
				for _, conditionOffset := range conditionOffsets {
					row, gotActionOffset, ok := decodeItemRentalAt(record, conditionOffset)
					if ok && gotActionOffset == actionOffset {
						candidates[row] = true
					}
				}
			}
		}
		if len(candidates) != 1 {
			return nil, fmt.Errorf("rental action at offset %d resolves to %d record frames", actionOffset, len(candidates))
		}
		for row := range candidates {
			rows = append(rows, row)
		}
	}

	return rows, nil
}

func decodeItemRentalAt(record []byte, offset int) (ItemRentalRow, int, bool) {
	c := bss.NewCursor(record, offset, len(record))
	condition := c.UTF16()
	name := c.UTF16()
	unknown0 := uint16(c.U16())
	variant := c.U16()
	if !c.OK() || name == "" || variant != 0 {
		return ItemRentalRow{}, 0, false
	}
	description := c.UTF16()
	actionOffset := c.Pos()
	action := c.UTF16()
	if !c.OK() || strings.ContainsRune(condition, '\x00') ||
		strings.ContainsRune(name, '\x00') || strings.ContainsRune(description, '\x00') ||
		strings.ContainsRune(action, '\x00') {
		return ItemRentalRow{}, 0, false
	}
	args, ok := parsePointItemAction(action)
	if !ok {
		return ItemRentalRow{}, 0, false
	}

	return ItemRentalRow{
		ItemKey:           args[0],
		ItemSubKey:        args[1],
		Count:             args[2],
		PointType:         args[3],
		PointCost:         args[4],
		ConditionDSL:      condition,
		SourceName:        name,
		SourceDescription: description,
		Unknown0:          unknown0,
	}, actionOffset, true
}

func parsePointItemAction(action string) ([5]uint32, bool) {
	var out [5]uint32
	action = strings.TrimSpace(action)
	action = strings.TrimSpace(strings.TrimSuffix(action, ";"))
	prefixLen := len(buyItemByPointAction)
	if len(action) <= prefixLen+2 ||
		!strings.EqualFold(action[:prefixLen], buyItemByPointAction) ||
		action[prefixLen] != '(' || action[len(action)-1] != ')' {
		return out, false
	}
	parts := strings.Split(action[prefixLen+1:len(action)-1], ",")
	if len(parts) != len(out) {
		return out, false
	}
	for i, part := range parts {
		value, err := strconv.ParseUint(strings.TrimSpace(part), 10, 32)
		if err != nil {
			return out, false
		}
		out[i] = uint32(value)
	}

	return out, true
}
