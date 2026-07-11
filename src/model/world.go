package model

import "github.com/idevelopthings/bdo-data-extractor/src/models"

// World is the consolidated geographic database (world.json): the territory list
// and every map region with its names, parent area, territory membership, world
// position and town connections. Monster-zone data stays in zones.json; NPC and
// monster spawn placements stay in regions.json.
//
// Name fields hold the game's embedded (Korean) string by default and are
// replaced with the localized text when the loc tables have it — so a missing
// translation degrades to the source string instead of an empty field.
type World struct {
	Territories []Territory   `json:"territories"`
	Regions     []WorldRegion `json:"regions"`
	Nodes       []WorldNode   `json:"nodes"`
}

// WorldNode is one worldmap node from exploration.bss (the node-manager
// network: towns, gateways, farms, forests, mines, …). Key matches loc table
// 29 (localized node names) and the node ids community sites use. Kind is the
// game's node-kind enum (drives the worldmap icon). Territory is DERIVED from
// the nearest region's territory (the client resolves it indirectly via the
// waypoint system; the raw record stores no territory). Node connections are
// not client-side.
type WorldNode struct {
	*models.BaseFor[WorldNode]

	Key       int        `json:"key"`
	Name      string     `json:"name"`
	Kind      int        `json:"kind"`
	Territory int        `json:"territory"`
	Position  [3]float64 `json:"position"`
}

// WorldRegion is one map region from regioninfo.bss: Velia, Heidel Pass, Evergart
// Falls, … Key matches loc table 17 (localized place names), the regionclientdata
// spawn regions and region_info.xml bounds. Territory indexes into
// World.Territories (Velia → 0 Balenos, Heidel → 1 Serendia, …); the territory's
// capital region lives on Territory.CapitalKey. Type 1 = major city, 2 = field.
type WorldRegion struct {
	*models.BaseFor[WorldRegion]

	Key       int        `json:"key"`
	Name      string     `json:"name"`
	Type      int        `json:"type"`
	Territory int        `json:"territory"`
	Position  [3]float64 `json:"position"`
	// ExtraPositions are additional worldmap mark placements for oversized
	// zones (only the Great Desert of Valencia carries them).
	ExtraPositions [][3]float64 `json:"extraPositions,omitempty"`
	// WarehouseGroup (only on the 58 warehouse-bearing places) lists the region
	// keys of every warehouse in this place's storage/transport group,
	// including itself. The groups are disjoint and match the game's transport
	// topology: one big mainland cluster, Valencia City ↔ Ancado Inner Harbor,
	// the Morning Land villages, and isolated storages listing only themselves
	// (Iliya Island, Lema Island, Arehaza, Oquilla's Eye).
	WarehouseGroup []int `json:"warehouseGroup,omitempty"`
	// VariantOf groups spawn-phase variants of the same place: the game keeps
	// one region record per phase (quest states, Day/Night, …), all sharing a
	// name and position — e.g. Ancient Stone Chamber is keys 26/137/155.
	// 0/absent = the place's canonical (lowest-key) record; otherwise the
	// canonical record's key. Phase records stay separate because spawn data
	// (regions.json) references the specific phase keys.
	VariantOf int `json:"variantOf,omitempty"`
}
