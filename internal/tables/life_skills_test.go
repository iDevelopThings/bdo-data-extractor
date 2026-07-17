package tables

import (
	"encoding/binary"
	"testing"

	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

func TestDecodeLifeSkillProgression(t *testing.T) {
	t.Parallel()

	typeCount := int(model.LifeSkillTypeCount)
	const levelCount = 181
	experienceData := binary.LittleEndian.AppendUint32(nil, uint32(typeCount))
	maxLevelData := make([]byte, 0, typeCount*4)
	for typeValue := 0; typeValue < typeCount; typeValue++ {
		experienceData = binary.LittleEndian.AppendUint32(experienceData, levelCount)
		maxLevelData = binary.LittleEndian.AppendUint32(maxLevelData, levelCount-1)
		for levelValue := 0; levelValue < levelCount; levelValue++ {
			experienceData = append(experienceData, byte(typeValue))
			experienceData = binary.LittleEndian.AppendUint32(experienceData, uint32(levelValue))
			experienceData = binary.LittleEndian.AppendUint64(experienceData, uint64(typeValue*1000+levelValue))
		}
	}

	progression, err := DecodeLifeSkillProgression(experienceData, maxLevelData)
	if err != nil {
		t.Fatal(err)
	}
	if len(progression) != typeCount {
		t.Fatalf("progression count = %d, want %d", len(progression), typeCount)
	}
	bartering := progression[model.LifeSkillTypeBartering]
	if bartering.Type != model.LifeSkillTypeBartering || bartering.MaxLevel != 180 {
		t.Fatalf("bartering progression = %+v", bartering)
	}
	if got := bartering.Levels[81].RequiredExperience; got != 11081 {
		t.Fatalf("bartering Guru 1 experience = %d, want 11081", got)
	}
}
