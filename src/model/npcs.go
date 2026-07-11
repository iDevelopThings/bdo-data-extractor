package model

import "github.com/idevelopthings/bdo-data-extractor/src/models"

// NPC is one row of npcsimply.bss: an NPC's id and its in-client name/title.
// Names are the client's own (Korean) strings; English names live in .loc and
// can be joined later by ID.
type NPC struct {
	*models.BaseFor[NPC]

	ID     uint32     `json:"id"`
	Name   string     `json:"name"`
	Title  string     `json:"title,omitempty"` // generic role label, e.g. "<Fruit Merchant>"
	Spawns []NPCSpawn `json:"spawns,omitempty"`
}

// NPCSpawn is one placement of an NPC: the region it spawns in (key + its
// topography name, e.g. "Calpheon City") and its world position.
type NPCSpawn struct {
	Region     uint32     `json:"region"`
	RegionName string     `json:"regionName,omitempty"` // loc table 17, keyed by region
	Pos        [3]float64 `json:"pos"`
}
