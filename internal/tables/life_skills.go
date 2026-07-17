package tables

import (
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

// DecodeLifeSkillProgression decodes lifeexp.dbss and lifeexpmaxlevel.bss.
func DecodeLifeSkillProgression(experienceData, maxLevelData []byte) ([]model.LifeSkillProgression, error) {
	typeCount := int(model.LifeSkillTypeCount)
	if len(maxLevelData) != typeCount*4 {
		return nil, fmt.Errorf("lifeexpmaxlevel: size %d, want %d", len(maxLevelData), typeCount*4)
	}

	c := bss.NewCursor(experienceData, 0, len(experienceData))
	if count := int(c.U32()); count != typeCount {
		return nil, fmt.Errorf("lifeexp: type count %d, want %d", count, typeCount)
	}

	out := make([]model.LifeSkillProgression, 0, typeCount)
	for typeValue := 0; typeValue < typeCount; typeValue++ {
		maxLevel := model.LifeSkillLevel(bss.U32(maxLevelData, typeValue*4))
		levelCount := int(c.U32())
		if !c.OK() || levelCount <= 0 || levelCount > c.Remaining()/13 {
			return nil, fmt.Errorf("lifeexp: type %d has invalid level count %d", typeValue, levelCount)
		}
		if levelCount != int(maxLevel)+1 {
			return nil, fmt.Errorf("lifeexp: type %d has %d levels, want %d", typeValue, levelCount, int(maxLevel)+1)
		}
		levels := make([]model.LifeSkillExperienceLevel, 0, levelCount)
		for levelValue := 0; levelValue < levelCount; levelValue++ {
			rowType := c.U8()
			rowLevel := c.U32()
			requiredExperience := c.U64()
			if !c.OK() {
				return nil, fmt.Errorf("lifeexp: type %d level %d is truncated", typeValue, levelValue)
			}
			if rowType != typeValue || rowLevel != uint32(levelValue) {
				return nil, fmt.Errorf(
					"lifeexp: type/level %d/%d resolves to %d/%d",
					typeValue,
					levelValue,
					rowType,
					rowLevel,
				)
			}
			levels = append(levels, model.LifeSkillExperienceLevel{
				Level:              model.LifeSkillLevel(rowLevel),
				RequiredExperience: requiredExperience,
			})
		}

		out = append(out, model.LifeSkillProgression{
			Type:     model.LifeSkillType(typeValue),
			MaxLevel: maxLevel,
			Levels:   levels,
		})
	}
	if !c.OK() || c.Remaining() != 0 {
		return nil, fmt.Errorf("lifeexp: %d trailing bytes", c.Remaining())
	}

	return out, nil
}
