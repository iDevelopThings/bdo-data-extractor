package tables

import (
	"fmt"
	"slices"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
)

// DecodeNodeManagerOwners resolves each manager character key to the one node
// that owns it. exploration.bss repeats the character key across every node in
// a managed family; characterfunction.dbss stores the same family as an ordered
// counted u32 list whose first node is the owner.
func DecodeNodeManagerOwners(
	indexData, data []byte,
	families map[uint32][]uint32,
) (map[uint32]uint32, error) {
	entries, err := bss.ParseU16OffsetIndex("characterfunction", indexData, len(data))
	if err != nil {
		return nil, fmt.Errorf("parse character-function index: %w", err)
	}
	entryByKey := make(map[uint32]bss.IndexEntry, len(entries))
	for _, entry := range entries {
		if _, exists := entryByKey[entry.Key]; exists {
			return nil, fmt.Errorf("duplicate character-function key %d", entry.Key)
		}
		entryByKey[entry.Key] = entry
	}

	owners := make(map[uint32]uint32, len(families))
	for characterKey, family := range families {
		if characterKey == 0 || len(family) == 0 {
			return nil, fmt.Errorf("invalid manager family character=%d nodes=%v", characterKey, family)
		}
		entry, exists := entryByKey[characterKey]
		if !exists {
			return nil, fmt.Errorf("manager character %d has no character-function record", characterKey)
		}
		record, ok := entry.Slice(data)
		if !ok {
			return nil, fmt.Errorf("manager character %d record is out of bounds", characterKey)
		}
		lists := matchingNodeLists(record, family)
		if len(lists) != 1 {
			return nil, fmt.Errorf(
				"manager character %d has %d matching node-family lists, want 1",
				characterKey, len(lists),
			)
		}
		owners[characterKey] = lists[0][0]
	}

	return owners, nil
}

func matchingNodeLists(record []byte, family []uint32) [][]uint32 {
	want := slices.Clone(family)
	slices.Sort(want)
	for i, nodeKey := range want {
		if nodeKey == 0 || i > 0 && nodeKey == want[i-1] {
			return nil
		}
	}

	var matches [][]uint32
	listSize := 4 + 4*len(family)
	for offset := 0; offset+listSize <= len(record); offset++ {
		if int(bss.U32(record, offset)) != len(family) {
			continue
		}
		got := make([]uint32, len(family))
		for i := range got {
			got[i] = bss.U32(record, offset+4+i*4)
		}
		sorted := slices.Clone(got)
		slices.Sort(sorted)
		if slices.Equal(sorted, want) {
			matches = append(matches, got)
		}
	}

	return matches
}
