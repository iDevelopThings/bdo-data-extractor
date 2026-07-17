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

	// Byte ranges that read all-zero across every record (+12/+13/+15, +55..+90, +95..+103) and the +14 const are
	// dropped — the decoder warns if that ever changes.

	// Enabled is false on unused, unlocalized node records.
	Enabled bool `json:"enabled"`
	// Unknown17 closely tracks Main but is false on three active main nodes.
	Unknown17 bool `json:"unknown17,omitempty"`
	// SubKey2 (a key at +27, == SubKey for 997/1037 nodes)
	SubKey2 int `json:"subKey2,omitempty"`
	// GroupHash is list-0 (a coarser grouping hash, 33 distinct values)
	GroupHash []int `json:"groupHash,omitempty"`
	Unknown2  int   `json:"unknown2,omitempty"`
	Unknown8  int   `json:"unknown8,omitempty"`

	// The +16..+22 bytes are a small "location/zone class" subsystem, mapped by testing values
	// against shrddr's dump and the in-game world map. The names are best-guess (the exact game
	// terms aren't confirmed. +18 is an exact copy of Main and is dropped.
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

// WorldRegionUnknowns preserves the typed but unidentified regioninfo fields.
// Header offsets are record-relative; tail offsets are relative to the fixed
// 171-byte block after the warehouse and extra-position lists.
type WorldRegionUnknowns struct {
	// Unknown11 is a small region-mode enum (observed 0, 1, 3 and 4).
	Unknown11 uint8 `json:"unknown11"`
	// Unknown12 is an early region capability flag.
	Unknown12 bool `json:"unknown12"`
	// Unknown13 is an early region capability flag.
	Unknown13 bool `json:"unknown13"`
	// Unknown18 is part of the environment/capability flag bank.
	Unknown18 bool `json:"unknown18"`
	// Unknown19 is part of the environment/capability flag bank.
	Unknown19 bool `json:"unknown19"`
	// Unknown20 is part of the environment/capability flag bank.
	Unknown20 bool `json:"unknown20"`
	// Unknown21 is part of the environment/capability flag bank.
	Unknown21 bool `json:"unknown21"`
	// Unknown22 is part of the environment/capability flag bank.
	Unknown22 bool `json:"unknown22"`
	// Unknown23 is part of the environment/capability flag bank.
	Unknown23 bool `json:"unknown23"`
	// Unknown24 is part of the environment/capability flag bank.
	Unknown24 bool `json:"unknown24"`
	// Unknown25 is part of the environment/capability flag bank.
	Unknown25 bool `json:"unknown25"`
	// Unknown26 is part of the environment/capability flag bank.
	Unknown26 bool `json:"unknown26"`
	// Unknown28 is a flag adjacent to the locator setting.
	Unknown28 bool `json:"unknown28"`
	// Unknown29 is a short locator/region configuration value.
	Unknown29 uint16 `json:"unknown29"`
	// Unknown31 is a locator-adjacent region flag.
	Unknown31 bool `json:"unknown31"`
	// Unknown32 is a client configuration token, constant in the sampled build.
	Unknown32 uint32 `json:"unknown32"`
	// Unknown37 is a flag adjacent to the primary respawn position.
	Unknown37 bool `json:"unknown37"`
	// Unknown54 is part of the respawn/configuration flag bank.
	Unknown54 bool `json:"unknown54"`
	// Unknown55 is part of the respawn/configuration flag bank.
	Unknown55 bool `json:"unknown55"`
	// Unknown56 is part of the respawn/configuration flag bank.
	Unknown56 bool `json:"unknown56"`
	// Unknown57 is part of the respawn/configuration flag bank.
	Unknown57 bool `json:"unknown57"`
	// Unknown58 is part of the respawn/configuration flag bank.
	Unknown58 bool `json:"unknown58"`
	// Unknown60 is a respawn/configuration reference or parameter.
	Unknown60 uint32 `json:"unknown60"`
	// Unknown66 qualifies the configuration value at offset 68.
	Unknown66 bool `json:"unknown66"`
	// Unknown68 is a region configuration reference or parameter.
	Unknown68 uint32 `json:"unknown68"`
	// Unknown82 qualifies the configuration value at offset 84.
	Unknown82 bool `json:"unknown82"`
	// Unknown84 is a region configuration reference or parameter.
	Unknown84 uint32 `json:"unknown84"`
	// Unknown107 is a relation key near the town and exploration references.
	Unknown107 uint16 `json:"unknown107"`
	// Unknown115 is a locality flag observed on Velia and Velia Beach.
	Unknown115 bool `json:"unknown115"`
	// Unknown147 is a flag preceding the client configuration block.
	Unknown147 bool `json:"unknown147"`
	// Unknown149 is an opaque client-build/configuration value.
	Unknown149 uint32 `json:"unknown149"`
	// Unknown153 holds five world/environment scalar parameters.
	Unknown153 [5]float64 `json:"unknown153"`
	// Unknown173 is a world/environment configuration reference.
	Unknown173 uint32 `json:"unknown173"`
	// Unknown177 is a world/environment configuration reference.
	Unknown177 uint32 `json:"unknown177"`
	// Unknown181 is a sentinel field (0xffffffff in the sampled build).
	Unknown181 uint32 `json:"unknown181"`
	// Unknown185 holds six optional town/configuration references.
	Unknown185 [6]uint32 `json:"unknown185"`
	// Unknown209 is the final flag in the variable record head.
	Unknown209 bool `json:"unknown209"`

	// UnknownTail1 is the first short parameter in the fixed tail.
	UnknownTail1 uint16 `json:"unknownTail1"`
	// UnknownTail3 is the second short parameter in the fixed tail.
	UnknownTail3 uint16 `json:"unknownTail3"`
	// UnknownTail5 is a world-space vector or three scalar parameters.
	UnknownTail5 [3]float64 `json:"unknownTail5"`
	// UnknownTail17 is a tail configuration reference or parameter.
	UnknownTail17 uint32 `json:"unknownTail17"`
	// UnknownTail21 is the first byte in a four-byte mode/flag group.
	UnknownTail21 uint8 `json:"unknownTail21"`
	// UnknownTail22 is the second byte in a four-byte mode/flag group.
	UnknownTail22 uint8 `json:"unknownTail22"`
	// UnknownTail23 is the third byte in a four-byte mode/flag group.
	UnknownTail23 uint8 `json:"unknownTail23"`
	// UnknownTail24 is the flag terminating the four-byte mode group.
	UnknownTail24 bool `json:"unknownTail24"`
	// UnknownTail25 holds two world-space vectors or six scalar parameters.
	UnknownTail25 [6]float64 `json:"unknownTail25"`
	// UnknownTail49 is an opaque 64-bit key or bitfield.
	UnknownTail49 uint64 `json:"unknownTail49"`
	// UnknownTail57 is a world/environment scalar parameter.
	UnknownTail57 float64 `json:"unknownTail57"`
	// UnknownTail61 is a world/environment scalar parameter.
	UnknownTail61 float64 `json:"unknownTail61"`
	// UnknownTail65 is a tail configuration reference or parameter.
	UnknownTail65 uint32 `json:"unknownTail65"`
	// UnknownTail69 is a tail capability flag.
	UnknownTail69 bool `json:"unknownTail69"`
	// UnknownTail70 is a tail capability flag.
	UnknownTail70 bool `json:"unknownTail70"`
	// UnknownTail71 is a tail configuration reference or parameter.
	UnknownTail71 uint32 `json:"unknownTail71"`
	// UnknownTail75 is a short tail configuration value.
	UnknownTail75 uint16 `json:"unknownTail75"`
	// UnknownTail77 is a short tail configuration value.
	UnknownTail77 uint16 `json:"unknownTail77"`
	// UnknownTail79 is a small mode or enum value.
	UnknownTail79 uint8 `json:"unknownTail79"`
	// UnknownTail81 is a world/environment scalar parameter.
	UnknownTail81 float64 `json:"unknownTail81"`
	// UnknownTail85 holds six optional configuration references.
	UnknownTail85 [6]uint32 `json:"unknownTail85"`
	// UnknownTail109 qualifies the following vector fields.
	UnknownTail109 bool `json:"unknownTail109"`
	// UnknownTail110 is a world-space vector or three scalar parameters.
	UnknownTail110 [3]float64 `json:"unknownTail110"`
	// UnknownTail122 is a world-space vector or three scalar parameters.
	UnknownTail122 [3]float64 `json:"unknownTail122"`
	// UnknownTail134 is the first byte in a short mode/flag group.
	UnknownTail134 uint8 `json:"unknownTail134"`
	// UnknownTail135 is the second byte in a short mode/flag group.
	UnknownTail135 uint8 `json:"unknownTail135"`
	// UnknownTail136 is a flag in the short mode group.
	UnknownTail136 bool `json:"unknownTail136"`
	// UnknownTail137 is the final byte in the short mode group.
	UnknownTail137 uint8 `json:"unknownTail137"`
	// UnknownTail145 is a small tail mode or enum value.
	UnknownTail145 uint8 `json:"unknownTail145"`
	// UnknownTail154 is a trailing configuration reference or parameter.
	UnknownTail154 uint32 `json:"unknownTail154"`
	// UnknownTail158 is a trailing configuration reference or parameter.
	UnknownTail158 uint32 `json:"unknownTail158"`
	// UnknownTail162 is a trailing configuration reference or parameter.
	UnknownTail162 uint32 `json:"unknownTail162"`
}

// WorldRegion is one map region from regioninfo.bss: Velia, Heidel Pass, Evergart
// Falls, … Key matches loc table 17 (localized place names), the regionclientdata
// spawn regions and region_info.xml bounds. Territory indexes into
// World.Territories (Velia → 0 Balenos, Heidel → 1 Serendia, …); the territory's
// capital region lives on Territory.CapitalKey. Type 1 = major city, 2 = field.
type WorldRegion struct {
	*models.BaseFor[WorldRegion]
	WorldRegionUnknowns

	Key  int    `json:"key"`
	Name string `json:"name"`
	Type int    `json:"type"`
	// MapColor is the record's RGB world-map color.
	MapColor [3]uint8 `json:"mapColor"`
	// VillageSiegeDay uses CppEnums.VillageSiegeType; 7 means no node-war day.
	VillageSiegeDay int `json:"villageSiegeDay"`
	// Ocean marks an open-ocean region.
	Ocean bool `json:"ocean"`
	// Desert marks a desert region.
	Desert bool `json:"desert"`
	// Prison marks a prison region.
	Prison bool `json:"prison"`
	// Sea marks a sea region.
	Sea bool `json:"sea"`
	// Locator reports whether the client locator includes this region.
	Locator bool `json:"locator"`
	// Territory is the world territory this region belongs to (urn::world:territory:<idx>).
	Territory *models.EntityRef[Territory] `json:"territory"`
	// AffiliatedTown is the town responsible for this region.
	AffiliatedTown *models.EntityRef[WorldRegion] `json:"affiliatedTown,omitempty"`
	// RegionGroupKey joins regiongroupinfo.bss.
	RegionGroupKey int `json:"regionGroupKey,omitempty"`
	// Exploration is the world-map node associated with this region.
	Exploration *models.EntityRef[WorldNode] `json:"exploration,omitempty"`
	// VillainRespawn is the fallback node used for outlaw/death respawns.
	VillainRespawn *models.EntityRef[WorldNode] `json:"villainRespawn,omitempty"`
	// VillainRespawnPosition is the position paired with VillainRespawn.
	VillainRespawnPosition [3]float64 `json:"villainRespawnPosition"`
	// WaypointPosition is the region's waypoint-interface position.
	WaypointPosition [3]float64 `json:"waypointPosition"`
	// Position is the region's world position.
	Position [3]float64 `json:"position"`
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
	// GuildWharfManager is the region's guild wharf service NPC.
	GuildWharfManager *models.EntityRef[NPC] `json:"guildWharfManager,omitempty"`
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
