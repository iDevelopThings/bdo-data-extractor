package build

import (
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
)

// buildLifeSkillProgression writes the client life-skill experience curves.
func (b *Builder) buildLifeSkillProgression() error {
	experienceData, err := b.src.Read("lifeexp.dbss")
	if err != nil {
		return err
	}
	maxLevelData, err := b.src.Read("lifeexpmaxlevel.bss")
	if err != nil {
		return err
	}
	progression, err := tables.DecodeLifeSkillProgression(experienceData, maxLevelData)
	if err != nil {
		return err
	}

	name, err := b.write("life_skill_progression.json", progression)
	if err != nil {
		return err
	}
	b.logf(fmt.Sprintf("life skills: %d progression curves -> %s", len(progression), name))

	return nil
}
