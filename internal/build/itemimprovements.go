package build

import (
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
	"github.com/idevelopthings/bdo-data-extractor/src/models"
	"github.com/idevelopthings/bdo-data-extractor/src/urn"
)

func (b *Builder) buildItemImprovements() error {
	data, err := b.src.Read("itemimprovement.dbss")
	if err != nil {
		return err
	}
	offsetRaw, err := b.src.Read("itemimprovementoffset.dbss")
	if err != nil {
		return err
	}
	raw, altCount, err := tables.DecodeItemImprovements(offsetRaw, data)
	if err != nil {
		return err
	}

	out := make([]model.ItemImprovement, 0, len(raw))
	for _, row := range raw {
		if b.items[row.Result] == nil {
			return fmt.Errorf("itemimprovement key %d: missing result item %d", row.Key, row.Result)
		}
		for _, baseID := range row.Bases {
			if b.items[baseID] == nil {
				return fmt.Errorf("itemimprovement key %d: missing base item %d", row.Key, baseID)
			}
		}

		out = append(out, model.ItemImprovement{
			Key:    row.Key,
			Result: model.ItemRef(row.Result),
			Bases:  model.ItemRefList(row.Bases...),
			Flag:   row.Flag,
		})

		// Sparse graph links on chain participants only. Multiple table rows and
		// multi-base rows can legitimately point at the same result.
		result := b.items[row.Result]
		if result.ReformsFrom == nil {
			result.ReformsFrom = &models.EntityRefList[model.Item]{}
		}
		for _, baseID := range row.Bases {
			result.ReformsFrom.AddUnique(urn.Item.New(baseID))
			base := b.items[baseID]
			if base.ReformsInto == nil {
				base.ReformsInto = &models.EntityRefList[model.Item]{}
			}
			base.ReformsInto.AddUnique(urn.Item.New(row.Result))
		}
	}

	path, err := b.addJSON("item_improvements.json", model.ItemImprovements{Rows: out})
	if err != nil {
		return err
	}
	b.logf(fmt.Sprintf("itemimprovement: %d reform rows, %d item-only rows validated -> %s", len(out), altCount, path))
	return nil
}
