package model

import "github.com/idevelopthings/bdo-data-extractor/src/models"

// Territory is one of the game's world territories (Balenos, Serendia, Calpheon,
// Mediah, …) in game order, decoded from territoryinfo.bss. Name/Nation hold the
// game's embedded (Korean) strings until the loc join replaces them (loc table
// 12, id == Index).
//
// Nation is the parent realm shown above the territory name — Balenos, Serendia
// and Calpheon all belong to the "Republic of Calpheon" (NationKey is that
// realm's shared hash). Primary marks the nation's direct/seat territory
// (칼페온 직할령 = direct-rule Calpheon), Autonomous the 자치령 territories
// (Balenos, Serendia). Positions are worldmap territory-mark placements.
// IconLarge/IconSmall are paths relative to the data dir, under
// icons/territories/ (written by the icons command).
// CrownItemID/ArmorItemID are the territory-conquest (siege) reward items;
// post-siege-era territories reuse Valencia's pair.
type Territory struct {
	*models.BaseFor[Territory]

	Index     int    `json:"index"`
	Name      string `json:"name"`
	Nation    string `json:"nation,omitempty"`
	NationKey uint32 `json:"nationKey"`
	// CapitalKey/CapitalName identify the territory's main region (Balenos →
	// Velia, Mediah → Altinova, …; every region record of the territory
	// carries this key, so it also distinguishes the two Edania territories).
	CapitalKey  int                     `json:"capitalKey,omitempty"`
	CapitalName string                  `json:"capitalName,omitempty"`
	Primary     bool                    `json:"primary,omitempty"`
	Autonomous  bool                    `json:"autonomous,omitempty"`
	Positions   [][3]float64            `json:"positions,omitempty"`
	IconLarge   string                  `json:"iconLarge,omitempty"`
	IconSmall   string                  `json:"iconSmall,omitempty"`
	CrownItemID *models.EntityRef[Item] `json:"crownItemId,omitempty"`
	ArmorItemID *models.EntityRef[Item] `json:"armorItemId,omitempty"`
	// ExtraKey is an optional per-territory key present only on Serendia (114)
	// and Mediah (312); not a region/node/loc id — meaning not yet identified.
	ExtraKey uint32 `json:"extraKey,omitempty"`
}
