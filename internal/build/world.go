package build

import (
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

// buildTerritories writes world.json — the consolidated geographic database:
// the 14 territories (territoryinfo.bss + loc table 12 English names) and all
// map regions (regioninfo.bss: name, parent area, territory membership, world
// position, town connections; English names from loc table 17 by region key).
func (b *Builder) buildTerritories() error {
	tData, err := b.src.Read("territoryinfo.bss")
	if err != nil {
		return err
	}
	terrs, err := tables.DecodeTerritories(tData)
	if err != nil {
		return err
	}
	for i := range terrs {
		id := uint32(terrs[i].Index)
		if en := b.gs.TerritoryNames[id]; en != "" {
			terrs[i].Name = en
		}
		if en := b.gs.MainCatNames[id]; en != "" {
			terrs[i].Nation = en
		}
		if terrs[i].IconLarge != "" {
			terrs[i].IconLarge = "icons/territories/" + tables.TerritoryIconFile(terrs[i].IconLarge)
		}
		if terrs[i].IconSmall != "" {
			terrs[i].IconSmall = "icons/territories/" + tables.TerritoryIconFile(terrs[i].IconSmall)
		}
	}

	rData, err := b.src.Read("regioninfo.bss")
	if err != nil {
		return err
	}
	regions, capitals, err := tables.DecodeRegionInfo(rData)
	if err != nil {
		return err
	}
	named := 0
	for i := range regions {
		if en := b.gs.Topography[uint32(regions[i].Key)]; en != "" {
			regions[i].Name = en
			named++
		}
	}
	markRegionVariants(regions)

	// each territory's capital region key is stored on its region records
	regionName := make(map[int]string, len(regions))
	for i := range regions {
		regionName[regions[i].Key] = regions[i].Name
	}
	for i := range terrs {
		if ck := capitals[terrs[i].Index]; ck != 0 {
			terrs[i].CapitalKey = ck
			terrs[i].CapitalName = regionName[ck]
		}
	}

	nData, err := b.src.Read("exploration.bss")
	if err != nil {
		return err
	}
	nodes, err := tables.DecodeExplorationNodes(nData)
	if err != nil {
		return err
	}
	nodesNamed := 0
	for i := range nodes {
		if en := b.gs.NodeNames[uint32(nodes[i].Key)]; en != "" {
			nodes[i].Name = en
			nodesNamed++
		}
		// the node record stores no territory (the client resolves it via the
		// waypoint system) — derive it from the nearest region's territory
		nodes[i].Territory = nearestRegionTerritory(regions, nodes[i].Position)
	}

	p, err := b.write("world.json", model.World{Territories: terrs, Regions: regions, Nodes: nodes})
	if err != nil {
		return err
	}
	b.logf(
		fmt.Sprintf(
			"world: %d territories, %d regions (%d named), %d nodes (%d named) -> %s",
			len(terrs), len(regions), named, len(nodes), nodesNamed, p,
		),
	)

	return nil
}

// markRegionVariants links spawn-phase variants of the same place: region
// records sharing a name AND position are one place with per-phase spawn sets
// (quest states, Day/Night, …). Each group's lowest key is the canonical
// record; the others get VariantOf pointing at it. Regions must be decoded in
// key order is NOT assumed — groups sort themselves.
func markRegionVariants(regions []model.WorldRegion) {
	type place struct {
		name string
		pos  [3]float64
	}
	groups := map[place][]int{} // -> indices into regions
	for i := range regions {
		p := place{regions[i].Name, regions[i].Position}
		groups[p] = append(groups[p], i)
	}
	for _, idxs := range groups {
		if len(idxs) < 2 {
			continue
		}
		canon := idxs[0]
		for _, i := range idxs[1:] {
			if regions[i].Key < regions[canon].Key {
				canon = i
			}
		}
		for _, i := range idxs {
			if i != canon {
				regions[i].VariantOf = regions[canon].Key
			}
		}
	}
}

// nearestRegionTerritory returns the territory of the region whose position is
// closest to pos (squared distance on x/z; y differences are elevation noise).
func nearestRegionTerritory(regions []model.WorldRegion, pos [3]float64) int {
	best, terr := -1.0, 0
	for i := range regions {
		if regions[i].Position == ([3]float64{}) {
			continue
		}
		dx := regions[i].Position[0] - pos[0]
		dz := regions[i].Position[2] - pos[2]
		d := dx*dx + dz*dz
		if best < 0 || d < best {
			best, terr = d, regions[i].Territory
		}
	}
	return terr
}

// buildWorld decodes NPCs and region/node spawn data, attaches world-space bounds
// and per-NPC spawn locations, and writes regions.json + npcs.json.
func (b *Builder) buildWorld() error {
	npcData, err := b.src.Read("npcsimply.bss")
	if err != nil {
		return err
	}
	npcs, err := tables.DecodeNPCs(npcData)
	if err != nil {
		return err
	}
	// npcsimply stores Korean names inline; prefer the English name from loc table 6.
	for i := range npcs {
		if en := b.gs.EntityNames[npcs[i].ID]; en != "" {
			npcs[i].Name = en
		}
	}

	// node/region spawn data: region -> NPC/monster placements + world positions.
	// Publisher variants are more complete than the base; fall back to base.
	regionXML, _, err := b.src.ReadAny("regionclientdata_en_.xml", "regionclientdata_na_.xml", "regionclientdata.xml")
	if err != nil {
		return err
	}
	regions, err := tables.DecodeRegions(regionXML)
	if err != nil {
		return err
	}
	// merge world-space bounds (region_info.xml, keyed by the same region id)
	if boundsXML, err := b.src.Read("region_info.xml"); err == nil {
		bounds := tables.DecodeRegionBounds(boundsXML)
		withBounds := 0
		for i := range regions {
			if bx := bounds[regions[i].Key]; bx != nil {
				regions[i].Bounds = bx
				withBounds++
			}
		}
		b.logf(fmt.Sprintf("region bounds: %d boxes-regions, attached to %d regions", len(bounds), withBounds))
	}
	rp, err := b.write("regions.json", regions)
	if err != nil {
		return err
	}
	b.logf(fmt.Sprintf("regions: %d (from regionclientdata) -> %s", len(regions), rp))

	// attach spawn locations to each NPC by character id; the region's place name
	// (loc table 17, keyed by region) gives the town/area.
	byChar := map[uint32][]model.NPCSpawn{}
	for _, r := range regions {
		name := b.gs.Topography[r.Key]
		for _, s := range r.Spawns {
			byChar[s.Key] = append(byChar[s.Key], model.NPCSpawn{Region: r.Key, RegionName: name, Pos: s.Pos})
		}
	}
	located := 0
	for i := range npcs {
		if sp := byChar[npcs[i].ID]; sp != nil {
			npcs[i].Spawns = sp
			located++
		}
	}
	np, err := b.write("npcs.json", npcs)
	if err != nil {
		return err
	}
	b.logf(fmt.Sprintf("npcs: %d (%d located) -> %s", len(npcs), located, np))

	b.regions = regions
	b.npcs = npcs

	return nil
}
