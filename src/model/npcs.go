package model

import "github.com/idevelopthings/bdo-data-extractor/src/models"

// NPC is an NPC character template keyed by the character key shared by
// npcsimply, characterspawntype, exploration and region spawn records. Spawns
// are its placed variants; one template can appear at several positions and
// dialog indices. Names are localized from loc table 6.
type NPC struct {
	*models.BaseFor[NPC]

	ID    uint32 `json:"id"`
	Name  string `json:"name"`
	Title string `json:"title,omitempty"` // generic role label, e.g. "<Fruit Merchant>"
	// SpawnTypes are the NPC's client-defined map/navigation roles. Explorer
	// identifies node managers; the other values identify town services.
	SpawnTypes NPCSpawnTypes `json:"spawnTypes,omitempty"`
	Spawns     []NPCSpawn    `json:"spawns,omitempty"`
}

// HasSpawnType reports whether the NPC has spawnType.
func (n NPC) HasSpawnType(spawnType NPCSpawnType) bool {
	return n.SpawnTypes.Has(spawnType)
}

// HasMapRole reports whether the NPC has a specialized map/navigation role.
func (n NPC) HasMapRole() bool {
	return n.SpawnTypes.HasMapRole()
}

// NPCSpawn is one placed variant of an NPC template: the region it spawns in
// (ref + key + its topography name), world position and dialog variant.
type NPCSpawn struct {
	Region     *models.EntityRef[WorldRegion] `json:"region,omitempty"`
	RegionKey  uint32                         `json:"regionKey"`
	RegionName string                         `json:"regionName,omitempty"` // loc table 17, keyed by region
	Pos        [3]float64                     `json:"pos"`
	// DialogIndex distinguishes multiple placed variants of the same character.
	DialogIndex int `json:"dialogIndex,omitempty"`
}
