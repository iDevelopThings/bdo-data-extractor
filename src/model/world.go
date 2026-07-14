package model

import "github.com/idevelopthings/bdo-data-extractor/src/models"

// World is the consolidated geographic database (world.json): the territory list
// and every map region with its names, parent area, territory membership, world
// position, town connections, world-space bounds and NPC/monster spawn
// placements. Monster-zone data stays in zones.json; per-NPC spawn locations are
// mirrored onto each NPC in npcs.json.
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
// game's node-kind enum (drives the worldmap icon). Position and Links come
// from mapdata_realexplore2.bwp; the remaining fields come from exploration.bss.
// Territory is derived from the nearest region because neither table stores it.
type WorldNode struct {
	*models.BaseFor[WorldNode]

	Key  int           `json:"key"`
	Name string        `json:"name"`
	Kind WorldNodeKind `json:"kind"`
	// Territory is the world territory this node sits in (urn::world:territory:<idx>),
	// derived from the nearest region because the node record stores no territory.
	Territory *models.EntityRef[Territory] `json:"territory"`
	Position  [3]float64                   `json:"position"`
	// ExplorationPosition is the family/label anchor at exploration.bss +104.
	// It is omitted when it already equals Position.
	ExplorationPosition *[3]float64 `json:"explorationPosition,omitempty"`

	// LinkedKey is a second node reference; == Key in every current record (a redundant copy).
	LinkedKey int `json:"linkedKey,omitempty"`
	// SubKey is the node's waypoint-space key.
	SubKey int `json:"subKey,omitempty"`
	// Knowledge references every knowledge entry the game associates with this node — its NPCs, creatures and topography (urn::knowledge:entry:<key>, see knowledge.json).
	Knowledge *models.EntityRefList[KnowledgeEntry] `json:"knowledge,omitempty"`

	// Main is the primary/sub node distinction (the byte at +116): true for towns, gateways,
	// farms and castles that carry knowledge; false for resource/sub nodes. Validated 100%
	// against bdolytics' `main` flag across 999 shared nodes.
	Main bool `json:"main"`

	// Contribution is the contribution-point cost to activate this node (0 for towns, else 1-3).
	Contribution int `json:"contribution,omitempty"`

	// Radius is the node's map influence radius (f32 at +31); +35 caches Radius² and is not
	// stored separately.
	Radius float64 `json:"radius,omitempty"`

	// Manager is the NPC template that manages this exact node. The build rejects
	// non-main kind-0 pseudo families and retains repeated character keys only on
	// the owner selected by characterfunction.dbss.
	Manager *models.EntityRef[NPC] `json:"manager,omitempty"`
	// ManagerNode points from an affiliated node to the node that owns its
	// manager. Resolve that node's Manager to reach the NPC template.
	ManagerNode *models.EntityRef[WorldNode] `json:"managerNode,omitempty"`
	// TownRepresentative is the ruler or representative stored at exploration.bss +45.
	TownRepresentative *models.EntityRef[NPC] `json:"townRepresentative,omitempty"`
	// Children are the non-main nodes directly connected to this main node in the
	// mapdata_realexplore2.bwp graph.
	Children *models.EntityRefList[WorldNode] `json:"children,omitempty"`
	// Links are the node's client-side graph edges from mapdata_realexplore2.bwp.
	Links *models.EntityRefList[WorldNode] `json:"links,omitempty"`
	// Products are the normal worker-production items available at this node.
	// Quantities and lucky bonus drops are not present in the client tables.
	Products *models.EntityRefList[Item] `json:"products,omitempty"`

	//  Byte ranges that read all-zero across every record (+12/+13/+15, +55..+90, +95..+103) and the +14 const are
	// dropped — the decoder warns if that ever changes.

	// Flag is the const-1 byte at +4;
	Flag int `json:"flag"`
	// SubKey2 (a key at +27, == SubKey for 997/1037 nodes)
	SubKey2 int `json:"subKey2,omitempty"`
	// GroupHash is list-0 (a coarser grouping hash, 33 distinct values)
	GroupHash []int `json:"groupHash,omitempty"`
	Unknown2  int   `json:"unknown2,omitempty"`
	Unknown8  int   `json:"unknown8,omitempty"`

	// The +16..+22 bytes are a small "location/zone class" subsystem, mapped by testing values
	// against shrddr's dump and the in-game world map. The names are best-guess (the exact game
	// terms aren't confirmed), and +17/+18 — which are just copies of Main — are dropped.
	//
	// Special marks the 119 non-network "special locations" (byte +16 == 0): every town
	// (kind 2), investment bank (kind 10), sea zone (Margoria/Ross/Juur), trade district and
	// battlefield. The other 918 nodes are standard, contribution-investable network nodes.
	Special bool `json:"special,omitempty"`
	// ZoneIndex is a unique index (7..247, byte +19) on 63 "special content" nodes — islands,
	// grind zones, castles, battlefields — a foreign key into an as-yet unidentified table.
	ZoneIndex int `json:"zoneIndex,omitempty"`
	// ZoneCategory classifies the ZoneIndex nodes (byte +20): 1 island · 2 coastal · 5 inland/
	// desert grind · 6 battlefield & ocean "Great Spot"; set on 47 of them.
	ZoneCategory int `json:"zoneCategory,omitempty"`
	// GrindZone marks the 25 monster grind zones (byte +21, the Marni/Elvia set: Manshaum,
	// Mirumok, Gyfin, Star's End, Hexe, …); the value is a unique index. Disjoint from zones.json
	// (the drop-UI hunting grounds).
	GrindZone int `json:"grindZone,omitempty"`
	// GrindTier is the difficulty tier (byte +22) of the 12 endgame grind zones the in-game world
	// map labels with a Recommended-AP value (2, or 3 for Star's End). A subset of GrindZone.
	GrindTier int `json:"grindTier,omitempty"`

	// Unknown39 is a small internal value (byte +39; 13 distinct: 1, 6, 10, 12, 18, 77, …) — still
	// unidentified.
	Unknown39 int `json:"unknown39,omitempty"`
	// NodeIndex is a per-node enumeration id (the low bits of +47; +47 also carries a constant
	// 0x20000 flag that is masked off here). 0 for sub-nodes; a near-sequential id on the ~229
	// main nodes, assigned roughly in region order (Balenos low, the islands 606-642 consecutive,
	// Valencia/Land of Morning Light high). Looks like the node's index into another table.
	NodeIndex int `json:"nodeIndex,omitempty"`
	// AreaID is a worldmap area/sector id (the high word of +51, which is stored as AreaID<<16).
	// 44 areas that group nodes geographically — the whole contiguous old-world landmass is one
	// 525-node sector, with islands, the Valencia desert and Land of Morning Light split into
	// their own. Finer than territory, coarser than region.
	AreaID int `json:"areaId,omitempty"`
}

// WorldRegion is one map region from regioninfo.bss: Velia, Heidel Pass, Evergart
// Falls, … Key matches loc table 17 (localized place names), the regionclientdata
// spawn regions and region_info.xml bounds. Territory indexes into
// World.Territories (Velia → 0 Balenos, Heidel → 1 Serendia, …); the territory's
// capital region lives on Territory.CapitalKey. Type 1 = major city, 2 = field.
type WorldRegion struct {
	*models.BaseFor[WorldRegion]

	Key  int    `json:"key"`
	Name string `json:"name"`
	Type int    `json:"type"`
	// Territory is the world territory this region belongs to (urn::world:territory:<idx>).
	Territory *models.EntityRef[Territory] `json:"territory"`
	Position  [3]float64                   `json:"position"`
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
	// references the specific phase keys.
	VariantOf int `json:"variantOf,omitempty"`
	// Bounds is the region's world-space AABB (union of its spatial boxes, from
	// region_info.xml); absent when the region has no box data.
	Bounds *Bounds `json:"bounds,omitempty"`
	// Spawns are the NPC/monster placements inside this region
	// (from regionclientdata.xml): character id, world position and dialog variant.
	Spawns []Spawn `json:"spawns,omitempty"`
}

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
