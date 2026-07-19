package build

import (
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
	"github.com/idevelopthings/bdo-data-extractor/src/models"
)

func (b *Builder) buildLightstoneCombinations(buffs map[uint16]tables.Buff, skills map[uint32]tables.SkillEffect) error {
	data, err := b.src.Read("lightstoneset.bss")
	if err != nil {
		return err
	}
	rows, rawAliases, err := tables.DecodeLightstoneCombinations(data)
	if err != nil {
		return err
	}

	combinations := make([]model.LightstoneCombination, 0, len(rows))
	for _, row := range rows {
		localized, ok := b.gs.LightstoneSets[row.Key]
		if !ok || localized.Name == "" || localized.Description == "" {
			return fmt.Errorf("lightstoneset: combination %d is absent from loc table 113", row.Key)
		}
		for _, itemID := range row.RequiredItems {
			if b.items[itemID] == nil {
				return fmt.Errorf("lightstoneset: combination %d requires missing item %d", row.Key, itemID)
			}
		}
		skill, ok := skills[row.SkillIndexKey()]
		if !ok || len(skill.Buffs) == 0 {
			return fmt.Errorf("lightstoneset: combination %d has unresolved skill %d", row.Key, row.SkillKey)
		}
		effects := b.buildEffects(buffs, skill)
		if effects == nil {
			return fmt.Errorf("lightstoneset: combination %d has no decoded effects", row.Key)
		}
		combinations = append(combinations, model.LightstoneCombination{
			BaseFor:     models.NewBaseFor[model.LightstoneCombination](row.Key),
			Key:         row.Key,
			Name:        localized.Name,
			Description: localized.Description,
			SkillKey:    row.SkillKey,
			Required:    model.ItemRefList(row.RequiredItems...),
			Effects:     effects,
		})
	}

	aliases := make([]model.LightstoneItemAlias, 0, len(rawAliases))
	unresolvedAliases := 0
	for _, alias := range rawAliases {
		resolved := model.LightstoneItemAlias{
			ItemID:     alias.Item,
			CountsAsID: alias.CountsAs,
		}
		if b.items[alias.Item] != nil {
			resolved.Item = model.ItemRef(alias.Item)
		}
		if b.items[alias.CountsAs] != nil {
			resolved.CountsAs = model.ItemRef(alias.CountsAs)
		}
		if resolved.Item == nil || resolved.CountsAs == nil {
			unresolvedAliases++
		}
		aliases = append(aliases, resolved)
	}

	if _, err := b.addJSON("lightstone_combinations.json", model.LightstoneData{
		Combinations: combinations,
		Aliases:      aliases,
	}); err != nil {
		return err
	}
	b.logf(fmt.Sprintf("lightstoneset: %d combinations, %d item aliases, %d aliases with unresolved items", len(combinations), len(aliases), unresolvedAliases))
	return nil
}
