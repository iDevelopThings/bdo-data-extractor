package model

import "github.com/idevelopthings/bdo-data-extractor/src/models"

// ItemSet is one skillpiece definition and the items linked to it.
type ItemSet struct {
	*models.BaseFor[ItemSet]

	SkillNo           uint32                      `json:"skillNo"`
	Bonuses           []ItemSetBonus              `json:"bonuses"`
	Items             *models.EntityRefList[Item] `json:"items,omitempty"`
	MembershipSources []ItemSetMembershipSource   `json:"membershipSources,omitempty"`
}

// ItemSetMembershipSource describes how an item-to-set relation was recovered.
type ItemSetMembershipSource string

const (
	// ItemSetMembershipDSL is a relation carried by an enhancement DSL function.
	ItemSetMembershipDSL ItemSetMembershipSource = "dsl"
	// ItemSetMembershipExplicit is a confirmed family with no recovered client-side foreign key.
	ItemSetMembershipExplicit ItemSetMembershipSource = "explicit"
)

// ItemSetBonus is one piece-count tier displayed by the client.
type ItemSetBonus struct {
	Pieces           uint32 `json:"pieces"`
	Apply            uint16 `json:"apply"`
	GroupTitle       string `json:"groupTitle,omitempty"`
	DescriptionTitle string `json:"descriptionTitle,omitempty"`
	Description      string `json:"description,omitempty"`
}
