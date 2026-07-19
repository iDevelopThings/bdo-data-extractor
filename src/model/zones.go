package model

import (
	"github.com/idevelopthings/bdo-data-extractor/src/models"
	"github.com/idevelopthings/bdo-data-extractor/src/urn"
)

// Zone is one monster/drop zone from dropuihuntinggroundinfo.bss — the data
// behind the in-game "Monster Zone Info" window. The table holds 105 records,
// one per zone, in name order. Each record is a packed sequence of length-
// prefixed sections followed by a fixed stat block; this struct is the decoded
// form.
type Zone struct {
	*models.BaseFor[Zone]

	Name            string                     `json:"name,omitempty"` // English (resolved from loc)
	Key             uint32                     `json:"key,omitempty"`  // huntingGroundKey
	MainCategory    *Category                  `json:"mainCategory,omitempty"`
	SubCategories   []Category                 `json:"subCategories,omitempty"`
	Node            *NodeRef                   `json:"node,omitempty"`
	SheetAP         int                        `json:"sheetAP,omitempty"`
	SheetDP         int                        `json:"sheetDP,omitempty"`
	TotalAP         int                        `json:"totalAP,omitempty"`
	TotalDP         int                        `json:"totalDP,omitempty"`
	EffectiveLimit  int                        `json:"effectiveLimit,omitempty"`
	ApplyPercent    int                        `json:"apApplyPercent,omitempty"`  // limited-AP apply percent
	Loot            models.EntityRefList[Item] `json:"loot,omitempty"`            // obtainable items -> items.json
	RecurringQuests []QuestRef                 `json:"recurringQuests,omitempty"` // repeat quests
	RegionQuests    []QuestRef                 `json:"regionQuests,omitempty"`    // region quests
	Titles          []Ref                      `json:"titles,omitempty"`          // earnable titles
	Tags            []TagInfo                  `json:"tags,omitempty"`            // zone tags (label, desc, colors)
	Ecology         []Ref                      `json:"ecology,omitempty"`         // ecology creature knowledge (loc table 6)
	Topography      []Ref                      `json:"topography,omitempty"`      // place/topography knowledge (loc table 17)
}

// Ref is a numeric id resolved to its display name (filled by the build from
// loc). Desc carries an optional description (e.g. a title's requirement text).
// URN is the durable link to the referenced entity when it maps to a catalog
// model (ecology → character, topography → world region); absent for refs with
// no backing model (e.g. titles).
type Ref struct {
	ID   uint32   `json:"id"`
	Name string   `json:"name,omitempty"`
	Desc string   `json:"desc,omitempty"`
	URN  *urn.URN `json:"urn,omitempty"`
}

// QuestRef is a "group-index" quest id resolved to its loc texts (filled by the build).
type QuestRef struct {
	ID         string           `json:"id"`
	Name       string           `json:"name,omitempty"`
	Desc       string           `json:"desc,omitempty"`
	Giver      string           `json:"giver,omitempty"`
	Objective  string           `json:"objective,omitempty"`
	Conditions *QuestConditions `json:"conditions,omitempty"`
}

// QuestConditions contains the client expressions evaluated before a quest can
// be accepted and when its objective is completed.
type QuestConditions struct {
	AcceptDSL   string `json:"acceptDsl,omitempty"`
	CompleteDSL string `json:"completeDsl,omitempty"`
}

// NodeRef is the zone's waypoint/node: the key (links the node graph), the zone
// name, and the nav position (present for nav-based zones).
type NodeRef struct {
	// Node resolves to the worldmap node in world.json, when the key is one (6 of the
	// 105 zones point at a key with no exploration node behind it).
	Node *models.EntityRef[WorldNode] `json:"urn,omitempty"`
	Key  uint32                       `json:"key"`
	Name string                       `json:"name,omitempty"`
	Pos  []float64                    `json:"pos,omitempty"`
}

// TagInfo is one Monster Zone Info tag from dropuitaginfo.bss: its key, label
// (filled from loc by the caller), and its UI colors.
type TagInfo struct {
	Key       uint32 `json:"key"`
	Name      string `json:"name,omitempty"`
	Desc      string `json:"desc,omitempty"`      // tooltip description (loc 117 desc field)
	Color     string `json:"color,omitempty"`     // texture/background color (ARGB)
	FontColor string `json:"fontColor,omitempty"` // text color (ARGB)
}

// Category is a Monster Zone Info main/sub category: its key, display name (from
// loc, filled by the build) and UI icon id (from the dropui*categoryinfo table).
type Category struct {
	ID   uint32 `json:"id"`
	Name string `json:"name,omitempty"`
	Icon string `json:"icon,omitempty"`
}
