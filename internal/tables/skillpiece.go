package tables

import (
	"fmt"
	"sort"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
	"github.com/idevelopthings/bdo-data-extractor/src/models"
)

const maxItemSetBonuses = 64

// DecodeItemSets reads skillpieceoffset.dbss and skillpiece.dbss.
func DecodeItemSets(offsetData, data []byte) ([]model.ItemSet, error) {
	sets := make([]model.ItemSet, 0)
	for rec, err := range bss.IndexedRecordsU16("skillpiece", offsetData, data) {
		if err != nil {
			return nil, err
		}
		set, err := decodeItemSet(rec.Entry.Key, rec.Data)
		if err != nil {
			return nil, err
		}
		sets = append(sets, set)
	}

	sort.Slice(sets, func(i, j int) bool {
		return sets[i].SkillNo < sets[j].SkillNo
	})
	return sets, nil
}

func decodeItemSet(indexKey uint32, record []byte) (model.ItemSet, error) {
	if len(record) < 16 {
		return model.ItemSet{}, fmt.Errorf("skillpiece: key %d record is truncated (%d bytes)", indexKey, len(record))
	}

	c := bss.NewCursor(record, 0, len(record))
	skillNo := c.U32()
	count := int(c.U32())
	if count <= 0 || count > maxItemSetBonuses {
		return model.ItemSet{}, fmt.Errorf("skillpiece: key %d has invalid bonus count %d", indexKey, count)
	}

	bonuses := make([]model.ItemSetBonus, 0, count)
	pieces := c.U32()
	for i := range count {
		if i > 0 {
			pieces = c.U32()
		}
		bonuses = append(bonuses, model.ItemSetBonus{
			Pieces:           pieces,
			Apply:            uint16(c.U16()),
			GroupTitle:       c.UTF16(),
			DescriptionTitle: c.UTF16(),
			Description:      c.UTF16(),
		})
	}
	footer := c.U32()
	if err := bss.RequireExhausted(c); err != nil {
		return model.ItemSet{}, fmt.Errorf("skillpiece: key %d: %w", indexKey, err)
	}
	switch {
	case skillNo != indexKey:
		return model.ItemSet{}, fmt.Errorf("skillpiece: index key %d resolves to skill %d", indexKey, skillNo)
	case footer != 0:
		return model.ItemSet{}, fmt.Errorf("skillpiece: key %d footer is %d, want 0", indexKey, footer)
	}

	return model.ItemSet{
		BaseFor: models.NewBaseFor[model.ItemSet](skillNo),
		SkillNo: skillNo,
		Bonuses: bonuses,
	}, nil
}
