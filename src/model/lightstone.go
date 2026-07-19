package model

import "github.com/idevelopthings/bdo-data-extractor/src/models"

// LightstoneData contains the client's combination effects and item aliases.
type LightstoneData struct {
	Combinations []LightstoneCombination `json:"combinations"`
	Aliases      []LightstoneItemAlias   `json:"aliases"`
}

// LightstoneCombination is a bonus activated by three or four infused items.
type LightstoneCombination struct {
	*models.BaseFor[LightstoneCombination]

	Key         uint32                     `json:"key"`
	Name        string                     `json:"name"`
	Description string                     `json:"description"`
	SkillKey    uint32                     `json:"skillKey"`
	Required    models.EntityRefList[Item] `json:"required"`
	Effects     *Effects                   `json:"effects,omitempty"`
}

// LightstoneItemAlias identifies an item that satisfies another lightstone's
// requirement in a combination.
type LightstoneItemAlias struct {
	ItemID     uint32                  `json:"itemId"`
	CountsAsID uint32                  `json:"countsAsId"`
	Item       *models.EntityRef[Item] `json:"item,omitempty"`
	CountsAs   *models.EntityRef[Item] `json:"countsAs,omitempty"`
}
