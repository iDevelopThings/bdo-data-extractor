package tables

import (
	"fmt"
	"sort"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
)

// PlantNodeProducts is the normal worker-production item set keyed by world
// node. UnresolvedNodes lists plant zones whose referenced item subgroup is not
// present in the current client data.
type PlantNodeProducts struct {
	ByNode          map[uint32][]uint32
	UnresolvedNodes []uint32
}

// DecodePlantNodeProducts joins the client worker-production tables into world
// node -> item ids. It intentionally excludes quantities and lucky bonus drops,
// which are not present in these client tables.
func DecodePlantNodeProducts(
	plantData, plantIndex, exchangeData, subgroupData, subgroupIndex []byte,
) (PlantNodeProducts, error) {
	productionByNode, err := decodePlantProductionKeys(plantData, plantIndex)
	if err != nil {
		return PlantNodeProducts{}, err
	}
	groupByProduction, err := decodePlantExchangeGroups(exchangeData)
	if err != nil {
		return PlantNodeProducts{}, err
	}
	subgroupEntries, err := bss.ParseU16OffsetIndex("itemsubgroup", subgroupIndex, len(subgroupData))
	if err != nil {
		return PlantNodeProducts{}, fmt.Errorf("parse item subgroup index: %w", err)
	}
	entryByGroup := make(map[uint32]bss.IndexEntry, len(subgroupEntries))
	for _, entry := range subgroupEntries {
		if _, exists := entryByGroup[entry.Key]; exists {
			return PlantNodeProducts{}, fmt.Errorf("duplicate item subgroup %d", entry.Key)
		}
		entryByGroup[entry.Key] = entry
	}

	result := PlantNodeProducts{ByNode: make(map[uint32][]uint32, len(productionByNode))}
	itemsByGroup := make(map[uint32][]uint32)
	for node, production := range productionByNode {
		group, exists := groupByProduction[production]
		if !exists {
			return PlantNodeProducts{}, fmt.Errorf("plant node %d references missing production key %d", node, production)
		}
		entry, exists := entryByGroup[group]
		if !exists {
			result.UnresolvedNodes = append(result.UnresolvedNodes, node)
			continue
		}
		items, exists := itemsByGroup[group]
		if !exists {
			items, err = decodeItemSubgroup(subgroupData, entry)
			if err != nil {
				return PlantNodeProducts{}, err
			}
			itemsByGroup[group] = items
		}
		if len(items) > 0 {
			result.ByNode[node] = items
		}
	}
	sort.Slice(result.UnresolvedNodes, func(i, j int) bool {
		return result.UnresolvedNodes[i] < result.UnresolvedNodes[j]
	})

	return result, nil
}

func decodePlantProductionKeys(data, index []byte) (map[uint32]uint32, error) {
	out := make(map[uint32]uint32)
	for rec, err := range bss.IndexedRecords(index, data) {
		if err != nil {
			return nil, fmt.Errorf("parse plant-zone index: %w", err)
		}
		c := bss.NewCursor(rec.Data, 0, len(rec.Data))
		key := c.U32()
		c.Skip(4 + 4 + 2 + 1 + 2 + 2 + 4)
		production := uint32(uint16(c.U32()))
		speciesCount := int(c.U32())
		if speciesCount < 0 || speciesCount > 32 {
			return nil, fmt.Errorf("plant-zone %d has invalid worker species count %d", rec.Entry.Key, speciesCount)
		}
		c.Skip(speciesCount)
		if err := bss.RequireExhausted(c); err != nil {
			return nil, fmt.Errorf("plant-zone %d: %w", rec.Entry.Key, err)
		}
		if key != rec.Entry.Key || key == 0 || production == 0 {
			return nil, fmt.Errorf("plant-zone index key %d does not match record key %d/production %d", rec.Entry.Key, key, production)
		}
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("duplicate plant-zone node %d", key)
		}
		out[key] = production
	}

	return out, nil
}

func decodePlantExchangeGroups(data []byte) (map[uint32]uint32, error) {
	p, err := bss.OpenPABR(data)
	if err != nil {
		return nil, fmt.Errorf("open plant exchange groups: %w", err)
	}
	if size, ok := p.RecordSize(); !ok || size != 94 {
		return nil, fmt.Errorf("plant exchange groups have record size %d, want 94", size)
	}
	out := make(map[uint32]uint32, p.Rows)
	for i := 0; i < p.Rows; i++ {
		start := p.RecordsStart + i*94
		c := bss.NewCursor(data, start, start+94)
		key := c.U16()
		duplicateKey := c.U16()
		c.Skip(2)
		group := c.U32()
		c.Skip(80)
		c.U32()
		if !c.OK() || c.Remaining() != 0 {
			return nil, fmt.Errorf("plant exchange row %d has invalid record size", i)
		}
		if key == 0 || key != duplicateKey || group == 0 {
			return nil, fmt.Errorf("plant exchange row %d has invalid keys %d/%d/group %d", i, key, duplicateKey, group)
		}
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("duplicate plant production key %d", key)
		}
		out[key] = group
	}

	return out, nil
}

func decodeItemSubgroup(data []byte, entry bss.IndexEntry) ([]uint32, error) {
	rec, ok := entry.Slice(data)
	if !ok {
		return nil, fmt.Errorf("item subgroup %d is out of bounds", entry.Key)
	}
	c := bss.NewCursor(rec, 0, len(rec))
	key := c.U32()
	c.Skip(10)
	count := int(c.U32())
	if count < 0 || count > 100 {
		return nil, fmt.Errorf("item subgroup %d has invalid item count %d", entry.Key, count)
	}
	items := make([]uint32, 0, count)
	for i := 0; i < count; i++ {
		item := c.U32()
		c.Skip(131)
		if item == 0 {
			return nil, fmt.Errorf("item subgroup %d contains a zero item id", entry.Key)
		}
		items = append(items, item)
	}
	if !c.OK() || c.Remaining() != 0 {
		return nil, fmt.Errorf("item subgroup %d has invalid record size", entry.Key)
	}
	if key != entry.Key {
		return nil, fmt.Errorf("item subgroup index key %d does not match record key %d", entry.Key, key)
	}

	return items, nil
}
