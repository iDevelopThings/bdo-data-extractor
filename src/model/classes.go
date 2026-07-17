package model

import "github.com/idevelopthings/bdo-data-extractor/src/models"

// IsPlayable reports whether c is a declared, non-reserved class type.
func (c CharacterClassType) IsPlayable() bool {
	info, ok := c.TryGetInfo()
	return ok && !info.Reserved
}

// CharacterGender is the player prototype's gender flag.
type CharacterGender string

const (
	CharacterGenderMale   CharacterGender = "male"
	CharacterGenderFemale CharacterGender = "female"
)

// ClassWeaponAsset is one class-selection weapon model and its weapon slot.
type ClassWeaponAsset struct {
	Slot int    `json:"slot"`
	Path string `json:"path"`
}

// CharacterClassUnknowns preserves the structured but unidentified pcgrowth
// fields. Header numbers use their record-relative byte offsets.
type CharacterClassUnknowns struct {
	Unknown4                 uint32    `json:"unknown4"`
	Unknown6                 uint32    `json:"unknown6"`
	Unknown8                 uint32    `json:"unknown8"`
	Unknown10                uint32    `json:"unknown10"`
	Unknown14                int       `json:"unknown14"`
	UnknownAvailability6     uint32    `json:"unknownAvailability6"`
	UnknownConfiguration0    []byte    `json:"unknownConfiguration0"`
	UnknownConfiguration94   int       `json:"unknownConfiguration94"`
	UnknownConfiguration95   []byte    `json:"unknownConfiguration95"`
	UnknownPresentation0     []float64 `json:"unknownPresentation0"`
	UnknownPresentation24    int       `json:"unknownPresentation24"`
	UnknownPresentation37    uint32    `json:"unknownPresentation37"`
	UnknownPresentation39    []float64 `json:"unknownPresentation39"`
	UnknownPresentation67    uint32    `json:"unknownPresentation67"`
	UnknownPresentationExtra []uint32  `json:"unknownPresentationExtra"`
}

// CharacterClass is one playable player class from the client growth tables.
type CharacterClass struct {
	CharacterClassUnknowns

	ClassType         CharacterClassType `json:"classType"`
	CharacterKey      uint32             `json:"characterKey"`
	Name              string             `json:"name,omitempty"`
	SourceName        string             `json:"sourceName,omitempty"`
	SourceDescription string             `json:"sourceDescription,omitempty"`
	Gender            CharacterGender    `json:"gender,omitempty"`
	// StarterWeapons are the low-tier weapon items listed for the class.
	StarterWeapons *models.EntityRefList[Item] `json:"starterWeapons,omitempty"`
	// PreviewWeapons are the main, sub, and awakening weapon items used by class presentation.
	PreviewWeapons    *models.EntityRefList[Item] `json:"previewWeapons,omitempty"`
	SelectionMovie    string                      `json:"selectionMovie,omitempty"`
	ConsumeAnimations []string                    `json:"consumeAnimations,omitempty"`
	WeaponAssetSets   [][]ClassWeaponAsset        `json:"weaponAssetSets,omitempty"`
}

// FitnessLevel is one Breath, Strength, or Health progression level.
type FitnessLevel struct {
	Level              int     `json:"level"`
	RequiredExperience uint32  `json:"requiredExperience"`
	MaxStamina         float64 `json:"maxStamina,omitempty"`
	MaxWeightLT        float64 `json:"maxWeightLT,omitempty"`
	MaxHP              float64 `json:"maxHP,omitempty"`
	MaxMP              float64 `json:"maxMP,omitempty"`
	Unknown9           float64 `json:"unknown9,omitempty"`
}

// CharacterLevelStat is the 200-byte character-stat block embedded in a level rule.
// Unknown offsets are relative to the start of that block at record offset +28.
type CharacterLevelStat struct {
	UnknownStat4   float64 `json:"unknownStat4,omitempty"`
	UnknownStat8   uint32  `json:"unknownStat8,omitempty"`
	UnknownStat32  float64 `json:"unknownStat32,omitempty"`
	UnknownStat60  uint32  `json:"unknownStat60,omitempty"`
	UnknownStat64  uint32  `json:"unknownStat64,omitempty"`
	UnknownStat68  uint32  `json:"unknownStat68,omitempty"`
	UnknownStat80  float64 `json:"unknownStat80,omitempty"`
	UnknownStat100 uint32  `json:"unknownStat100,omitempty"`
	UnknownStat104 uint32  `json:"unknownStat104,omitempty"`
	UnknownStat108 uint32  `json:"unknownStat108,omitempty"`
	UnknownStat112 float64 `json:"unknownStat112,omitempty"`
	APBonus        float64 `json:"apBonus,omitempty"`
	DPBonus        float64 `json:"dpBonus,omitempty"`
}

// CharacterLevelRule is one class and character-level entry from experience.bss.
type CharacterLevelRule struct {
	CharacterLevelStat

	ClassType CharacterClassType `json:"classType"`
	Level     int                `json:"level"`
}

// CharacterProgression is the client-side playable class and family-wide
// fitness data.
type CharacterProgression struct {
	Classes    []CharacterClass     `json:"classes"`
	LevelRules []CharacterLevelRule `json:"levelRules"`
	Breath     []FitnessLevel       `json:"breath"`
	Strength   []FitnessLevel       `json:"strength"`
	Health     []FitnessLevel       `json:"health"`
}
