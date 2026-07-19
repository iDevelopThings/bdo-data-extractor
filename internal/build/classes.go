package build

import (
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

// buildCharacterProgression registers the playable class identity map and the
// character-level rules and family-wide Breath, Strength, and Health curves.
func (b *Builder) buildCharacterProgression() error {
	var progression model.CharacterProgression

	simply, simplyErr := b.src.Read("pcgrowthsimply.bss")
	growthOffset, growthOffsetErr := b.src.Read("pcgrowthoffset.dbss")
	growth, growthErr := b.src.Read("pcgrowth.dbss")
	if simplyErr == nil && growthOffsetErr == nil && growthErr == nil {
		availability, err := tables.DecodeClassAvailability(simply)
		if err != nil {
			return err
		}
		availabilityByType := make(map[model.CharacterClassType]tables.ClassAvailability, len(availability))
		for _, class := range availability {
			availabilityByType[class.ClassType] = class
		}
		classes, err := tables.DecodeClassGrowth(growthOffset, growth)
		if err != nil {
			return err
		}
		for _, class := range classes {
			availability, ok := availabilityByType[class.ClassType]
			if !ok {
				return fmt.Errorf("pcgrowth: class %d is absent from pcgrowthsimply", class.ClassType)
			}
			if availability.SourceName != class.SourceName {
				return fmt.Errorf("pcgrowth: class %d source names differ: %q and %q", class.ClassType, availability.SourceName, class.SourceName)
			}
			if !availability.Playable {
				continue
			}
			starterWeapons := model.ItemRefList(class.StarterWeapons...)
			previewWeapons := model.ItemRefList(class.PreviewWeapons...)
			progression.Classes = append(progression.Classes, model.CharacterClass{
				CharacterClassUnknowns: class.Unknowns,
				ClassType:              class.ClassType,
				CharacterKey:           class.CharacterKey,
				Name:                   b.gs.EntityNames[class.CharacterKey],
				SourceName:             class.SourceName,
				SourceDescription:      class.SourceDescription,
				Gender:                 class.Gender,
				StarterWeapons:         &starterWeapons,
				PreviewWeapons:         &previewWeapons,
				SelectionMovie:         class.SelectionMovie,
				ConsumeAnimations:      class.ConsumeAnimations,
				WeaponAssetSets:        class.WeaponAssetSets,
			})
			progression.Classes[len(progression.Classes)-1].UnknownAvailability6 = availability.Unknown6
		}
	}

	experience, experienceErr := b.src.Read("experience.bss")
	if experienceErr == nil {
		levelRules, err := tables.DecodeCharacterLevelRules(experience)
		if err != nil {
			return err
		}
		progression.LevelRules = levelRules
	}

	fitnessOffset, fitnessOffsetErr := b.src.Read("fitnessleveloffset.dbss")
	fitness, fitnessErr := b.src.Read("fitnesslevel.dbss")
	if fitnessOffsetErr == nil && fitnessErr == nil {
		curves, err := tables.DecodeFitnessLevels(fitnessOffset, fitness)
		if err != nil {
			return err
		}
		progression.Breath = curves[0]
		progression.Strength = curves[1]
		progression.Health = curves[2]
	}

	if len(progression.Classes)+len(progression.LevelRules)+len(progression.Breath)+len(progression.Strength)+len(progression.Health) == 0 {
		return nil
	}
	p, err := b.addJSON("character_progression.json", progression)
	if err != nil {
		return err
	}
	b.logf(fmt.Sprintf(
		"character progression: %d playable classes, %d level rules, %d fitness levels -> %s",
		len(progression.Classes), len(progression.LevelRules), len(progression.Breath)+len(progression.Strength)+len(progression.Health), p,
	))
	return nil
}
