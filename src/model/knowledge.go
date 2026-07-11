package model

import "github.com/idevelopthings/bdo-data-extractor/src/models"

// KnowledgeTheme is one node of the knowledge category tree (mentaltheme.dbss) —
// e.g. "Ecology" or "Residents of Velia". Ecology themes fall in the 10001..20000
// key range. Item is the "You can learn about …" item whose name matches this
// theme (a whole-category knowledge item), if any.
type KnowledgeTheme struct {
	*models.BaseFor[KnowledgeTheme]

	Key    uint32                            `json:"key"`
	Name   string                            `json:"name,omitempty"`
	Parent *models.EntityRef[KnowledgeTheme] `json:"parent,omitempty"`
	Item   *models.EntityRef[Item]           `json:"item,omitempty"`
}

// KnowledgeEntry is one knowledge card (mentalcard.dbss): an entry under a theme,
// with the favor/interest values used by the amity minigame. Item is the
// single-entry knowledge item whose name matches this entry, if any.
type KnowledgeEntry struct {
	*models.BaseFor[KnowledgeEntry]

	Key         uint32                            `json:"key"`
	Theme       *models.EntityRef[KnowledgeTheme] `json:"theme,omitempty"`
	Name        string                            `json:"name,omitempty"`
	Description string                            `json:"description,omitempty"`
	Acquisition string                            `json:"acquisition,omitempty"` // how the entry is obtained (loc table 34, column 2)
	Image       string                            `json:"image,omitempty"`
	MinFavor    float64                           `json:"minFavor,omitempty"`
	MaxFavor    float64                           `json:"maxFavor,omitempty"`
	Interest    float64                           `json:"interest,omitempty"`
	Item        *models.EntityRef[Item]           `json:"item,omitempty"`
	// Character is the loc-6 entity (NPC/monster/statue/place) this entry is
	// about, matched by name; keyed by name slug (urn::character:<slug>).
	Character *models.EntityRef[Character] `json:"character,omitempty"`

	// Unknown20 (@20) is a flags bitfield we've mapped positionally but not yet
	// identified — obtain/display flags (its 18 values spread evenly across every
	// category, so it is NOT the subject kind; the kind is the theme category:
	// Characters / Ecology / Topography / Trade / …). Deviation-only, default 4.
	// (The @24–39 region after it is a packed sub-structure — u32 reads there come
	// back as `N<<8`, the item-RE misalignment tell — so it's read through, not
	// fielded, until its byte widths are cracked.)
	Unknown20 *int `json:"unknown20,omitempty"`
}

func (k *KnowledgeEntry) FavourRange() (minV, midV, maxV float64) {
	minV = k.MinFavor
	maxV = k.MaxFavor
	midV = (minV + maxV) / 2
	return
}
