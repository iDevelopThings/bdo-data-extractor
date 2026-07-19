package tables

import (
	"fmt"
	"sort"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

const characterSpawnTypeRecordSize = 48

// DecodeCharacterSpawnTypes decodes characterspawntype.dbss and its offset
// companion into character id -> enabled client SpawnType roles. Each record is
// [u16 character id][46 x u8 flags].
func DecodeCharacterSpawnTypes(indexData, data []byte) (map[uint32]model.NPCSpawnTypes, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("character spawn-type data is too small")
	}
	entries, err := bss.ParseU16OffsetIndex("characterspawntype", indexData, len(data))
	if err != nil {
		return nil, fmt.Errorf("parse character spawn-type index: %w", err)
	}
	count := int(bss.U32(data, 0))
	if count != len(entries) || len(data) != 4+count*characterSpawnTypeRecordSize {
		return nil, fmt.Errorf("character spawn-type count/size mismatch: count=%d index=%d data=%d", count, len(entries), len(data))
	}

	ordered := append([]bss.IndexEntry(nil), entries...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Offset < ordered[j].Offset
	})
	expectedOffset := uint32(4)
	for _, entry := range ordered {
		if entry.Offset != expectedOffset || entry.Size != characterSpawnTypeRecordSize {
			return nil, fmt.Errorf("character spawn-type record %d does not tile at offset %d", entry.Key, expectedOffset)
		}
		expectedOffset += entry.Size
	}

	out := make(map[uint32]model.NPCSpawnTypes, count)
	for rec, err := range bss.RecordsFromEntries(entries, data) {
		if err != nil {
			return nil, fmt.Errorf("character spawn-type record %d: %w", rec.Entry.Key, err)
		}
		if len(rec.Data) != characterSpawnTypeRecordSize {
			return nil, fmt.Errorf("character spawn-type record %d is out of bounds", rec.Entry.Key)
		}
		key := uint32(bss.U16(rec.Data, 0))
		if key == 0 || key != rec.Entry.Key {
			return nil, fmt.Errorf("character spawn-type index key %d does not match record key %d", rec.Entry.Key, key)
		}
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("duplicate character spawn-type key %d", key)
		}
		roles := make(model.NPCSpawnTypes, 0)
		for i, enabled := range rec.Data[2:] {
			if enabled > 1 {
				return nil, fmt.Errorf("character %d spawn type %d has invalid flag %d", key, i, enabled)
			}
			if enabled == 0 {
				continue
			}
			roles = append(roles, model.NPCSpawnType(i))
		}
		out[key] = roles
	}

	return out, nil
}
