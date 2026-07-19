package tables

import (
	"fmt"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
)

// CharacterItemServiceRow is the item-service prefix of one characterfunction record.
type CharacterItemServiceRow struct {
	CharacterKey uint32
	SourceName   string
	ConditionDSL string
	Unknown0     uint16
	UnknownKey   uint16
}

// DecodeCharacterItemServices reads the first character-function module, used for NPC
// shops, exchanges, contracts and similar item services. Rows with an empty
// module name have no active item service and are omitted.
func DecodeCharacterItemServices(offsetData, data []byte) (map[uint32]CharacterItemServiceRow, error) {
	entries, err := bss.ParseU16OffsetIndex("characterfunction", offsetData, len(data))
	if err != nil {
		return nil, fmt.Errorf("character function index: %w", err)
	}

	rows := make(map[uint32]CharacterItemServiceRow)
	for _, entry := range entries {
		record, ok := entry.Slice(data)
		if !ok {
			return nil, fmt.Errorf("character function %d is out of bounds", entry.Key)
		}
		if len(record) < 4 {
			return nil, fmt.Errorf("character function %d is truncated", entry.Key)
		}
		switch tag := bss.U16(record, 2); tag {
		case 0, 0x0100:
			continue
		case 0x0600:
		default:
			return nil, fmt.Errorf("character function %d has unknown prefix tag %#04x", entry.Key, tag)
		}
		c := bss.NewCursor(record, 4, len(record))
		name := c.UTF16()
		condition := c.UTF16()
		unknownKey := uint16(c.U16())
		if !c.OK() {
			return nil, fmt.Errorf("character function %d has a truncated item-service module", entry.Key)
		}
		if strings.ContainsRune(name, '\x00') || strings.ContainsRune(condition, '\x00') {
			return nil, fmt.Errorf("character function %d has an invalid item-service string", entry.Key)
		}
		if name == "" {
			continue
		}
		rows[entry.Key] = CharacterItemServiceRow{
			CharacterKey: entry.Key,
			SourceName:   name,
			ConditionDSL: condition,
			Unknown0:     bss.U16(record, 0),
			UnknownKey:   unknownKey,
		}
	}

	return rows, nil
}
