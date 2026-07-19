package build

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
	"github.com/idevelopthings/bdo-data-extractor/src/models"
	"github.com/idevelopthings/bdo-data-extractor/src/urn"
)

// buildWorld registers world.json — the consolidated geographic database: the 14
// territories (territoryinfo.bss + loc table 12 English names), all map regions
// (regioninfo.bss: name, parent area, territory membership, world position;
// English names from loc table 17 by region key) with their world-space bounds
// (region_info.xml) and NPC/monster spawn placements (regionclientdata.xml), and
// all exploration nodes. The decoded regions are kept on the Builder for the NPC
// and fishing stages.
func (b *Builder) buildWorld() error {
	tData, err := b.src.Read("territoryinfo.bss")
	if err != nil {
		return err
	}
	terrs, err := tables.DecodeTerritories(tData)
	if err != nil {
		return err
	}
	var iconRaws []string
	for i := range terrs {
		iconRaws = append(iconRaws, terrs[i].IconLarge, terrs[i].IconSmall)
	}
	iconNames := tables.TerritoryIconFiles(iconRaws)
	for i := range terrs {
		id := uint32(terrs[i].Index)
		if en := b.gs.TerritoryNames[id]; en != "" {
			terrs[i].Name = en
		}
		if en := b.gs.MainCatNames[id]; en != "" {
			terrs[i].Nation = en
		}
		if terrs[i].IconLarge != "" {
			terrs[i].IconLarge = "icons/territories/" + iconNames[terrs[i].IconLarge]
		}
		if terrs[i].IconSmall != "" {
			terrs[i].IconSmall = "icons/territories/" + iconNames[terrs[i].IconSmall]
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

	nodes, err := b.explorationTable()
	if err != nil {
		return err
	}
	managerOwners, managerAffiliates, err := b.resolveNodeManagerOwners(nodes)
	if err != nil {
		return err
	}
	plantData, err := b.src.Read("plantzone.dbss")
	if err != nil {
		return err
	}
	plantIndex, err := b.src.Read("plantzoneoffset.dbss")
	if err != nil {
		return err
	}
	exchangeData, err := b.src.Read("plantexchangegroup.bss")
	if err != nil {
		return err
	}
	subgroupData, err := b.src.Read("itemsubgroup.dbss")
	if err != nil {
		return err
	}
	subgroupIndex, err := b.src.Read("itemsubgroupoffset.dbss")
	if err != nil {
		return err
	}
	products, err := tables.DecodePlantNodeProducts(
		plantData, plantIndex, exchangeData, subgroupData, subgroupIndex,
	)
	if err != nil {
		return err
	}
	productNodes, productRefs := 0, 0
	for i := range nodes {
		ids := products.ByNode[uint32(nodes[i].Key)]
		if len(ids) == 0 {
			continue
		}
		for _, id := range ids {
			if b.items[id] == nil {
				return fmt.Errorf("world node %d references missing product item %d", nodes[i].Key, id)
			}
		}
		refs := model.ItemRefList(ids...)
		nodes[i].Products = &refs
		productNodes++
		productRefs += len(ids)
	}
	waypoints, err := b.waypointTable()
	if err != nil {
		return err
	}
	nodeByKey := make(map[int]int, len(nodes))
	for i := range nodes {
		nodeByKey[nodes[i].Key] = i
	}
	waypointPositions, waypointLinks := 0, 0
	for i := range nodes {
		waypoint, ok := waypoints[uint32(nodes[i].Key)]
		if !ok {
			continue
		}
		if nodes[i].Position != waypoint.Position {
			nodes[i].ExplorationPosition = new(nodes[i].Position)
		}
		nodes[i].Position = waypoint.Position
		waypointPositions++

		links := make([]urn.URN, 0, len(waypoint.Links))
		children := make([]urn.URN, 0, len(waypoint.Links))
		for _, key := range waypoint.Links {
			childIndex, exists := nodeByKey[int(key)]
			if !exists {
				continue
			}
			ref := urn.World.New("node", key)
			links = append(links, ref)
			if nodes[i].Main && !nodes[childIndex].Main {
				children = append(children, ref)
			}
		}
		if len(links) > 0 {
			nodes[i].Links = new(models.NewEntityRefList[model.WorldNode](links...))
			waypointLinks += len(links)
		}
		if len(children) > 0 {
			nodes[i].Children = new(models.NewEntityRefList[model.WorldNode](children...))
		}
	}
	nodesNamed := 0
	for i := range nodes {
		if en := b.gs.NodeNames[uint32(nodes[i].Key)]; en != "" {
			nodes[i].Name = en
			nodesNamed++
		}
		// the node record stores no territory (the client resolves it via the
		// waypoint system) — derive it from the nearest region's territory
		nodes[i].Territory = model.TerritoryRef(nearestRegionTerritory(regions, nodes[i].Position))
	}

	// Spawn data is layered by RegionInfo key: common data, the resource/language
	// baseline, then the service-region override. A later layer replaces the whole
	// region so removed or moved placements are not retained from an earlier layer.
	regionFiles := []string{
		"regionclientdata.xml",
		fmt.Sprintf("regionclientdata_%s_.xml", strings.ToLower(b.lang)),
	}
	if b.region != "" && !strings.EqualFold(b.region, b.lang) {
		regionFiles = append(regionFiles, fmt.Sprintf("regionclientdata_%s_.xml", strings.ToLower(b.region)))
	}
	spawnsByKey := map[uint32][]model.Spawn{}
	loadedRegionFiles := make([]string, 0, len(regionFiles))
	for _, regionFile := range regionFiles {
		regionData, exists, err := b.src.ReadIfExists(regionFile)
		if err != nil {
			return fmt.Errorf("read %s: %w", regionFile, err)
		}
		if !exists {
			continue
		}
		layer, err := tables.DecodeRegions(regionData)
		if err != nil {
			return fmt.Errorf("decode %s: %w", regionFile, err)
		}
		overlayRegionSpawns(spawnsByKey, layer)
		loadedRegionFiles = append(loadedRegionFiles, regionFile)
	}
	if len(loadedRegionFiles) == 0 {
		return fmt.Errorf("no region spawn data found for language %q and region %q", b.lang, b.region)
	}
	spawnCount := 0
	for i := range regions {
		if sp := spawnsByKey[uint32(regions[i].Key)]; sp != nil {
			regions[i].Spawns = sp
			spawnCount += len(sp)
		}
	}
	withBounds := 0
	if boundsXML, err := b.src.Read("region_info.xml"); err == nil {
		bounds := tables.DecodeRegionBounds(boundsXML)
		for i := range regions {
			if bx := bounds[uint32(regions[i].Key)]; bx != nil {
				regions[i].Bounds = bx
				withBounds++
			}
		}
	}
	b.regions = regions
	b.logf(fmt.Sprintf("region spawns: %s", strings.Join(loadedRegionFiles, " -> ")))

	p, err := b.addJSON("world.json", model.World{Territories: terrs, Regions: regions, Nodes: nodes})
	if err != nil {
		return err
	}
	b.logf(
		fmt.Sprintf(
			"world: %d territories, %d regions (%d named, %d spawns, %d with bounds), %d nodes (%d named, %d waypoint positions, %d links, %d manager owners/%d affiliates, %d product nodes/%d refs, %d unresolved) -> %s",
			len(terrs), len(regions), named, spawnCount, withBounds, len(nodes), nodesNamed, waypointPositions, waypointLinks, managerOwners, managerAffiliates, productNodes, productRefs, len(products.UnresolvedNodes), p,
		),
	)

	return nil
}

func overlayRegionSpawns(dst, layer map[uint32][]model.Spawn) {
	for key, spawns := range layer {
		dst[key] = spawns
	}
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
			best, terr = d, int(regions[i].Territory.ID())
		}
	}
	return terr
}

func (b *Builder) resolveNodeManagerOwners(nodes []model.WorldNode) (int, int, error) {
	families := nodeManagerFamilies(nodes)
	indexData, err := b.src.Read("characterfunctionoffset.dbss")
	if err != nil {
		return 0, 0, err
	}
	data, err := b.src.Read("characterfunction.dbss")
	if err != nil {
		return 0, 0, err
	}
	owners, err := tables.DecodeNodeManagerOwners(indexData, data, families)
	if err != nil {
		return 0, 0, err
	}

	ownerCount, affiliateCount, err := normalizeNodeManagers(nodes, owners)
	if err != nil {
		return 0, 0, err
	}
	return ownerCount, affiliateCount, nil
}

func nodeManagerFamilies(nodes []model.WorldNode) map[uint32][]uint32 {
	indices := make(map[uint32][]int)
	for i := range nodes {
		if nodes[i].Manager != nil {
			indices[nodes[i].Manager.ID()] = append(indices[nodes[i].Manager.ID()], i)
		}
	}

	families := make(map[uint32][]uint32, len(indices))
	for characterKey, familyIndices := range indices {
		ownsNode := false
		for _, i := range familyIndices {
			// Non-main normal families are pseudo zones; standalone kind-4 farms
			// are the only production nodes that directly own a manager.
			if nodes[i].Main || nodes[i].Kind == model.WorldNodeKindFarm {
				ownsNode = true
				break
			}
		}
		if !ownsNode {
			for _, i := range familyIndices {
				nodes[i].Manager = nil
			}
			continue
		}
		family := make([]uint32, 0, len(familyIndices))
		for _, i := range familyIndices {
			family = append(family, uint32(nodes[i].Key))
		}
		families[characterKey] = family
	}

	return families
}

func normalizeNodeManagers(nodes []model.WorldNode, owners map[uint32]uint32) (int, int, error) {
	nodeByKey := make(map[uint32]int, len(nodes))
	for i := range nodes {
		nodeByKey[uint32(nodes[i].Key)] = i
	}
	for characterKey, ownerKey := range owners {
		ownerIndex, exists := nodeByKey[ownerKey]
		if !exists {
			return 0, 0, fmt.Errorf("manager character %d owns missing node %d", characterKey, ownerKey)
		}
		if nodes[ownerIndex].Manager == nil || nodes[ownerIndex].Manager.ID() != characterKey {
			return 0, 0, fmt.Errorf("manager character %d does not match owner node %d", characterKey, ownerKey)
		}
	}

	ownerCount, affiliateCount := 0, 0
	for i := range nodes {
		if nodes[i].Manager == nil {
			continue
		}
		characterKey := nodes[i].Manager.ID()
		ownerKey, exists := owners[characterKey]
		if !exists {
			return 0, 0, fmt.Errorf("manager character %d has no owning node", characterKey)
		}
		if uint32(nodes[i].Key) == ownerKey {
			ownerCount++
			continue
		}
		nodes[i].Manager = nil
		nodes[i].ManagerNode = model.WorldNodeRef(ownerKey)
		affiliateCount++
	}

	return ownerCount, affiliateCount, nil
}

// explorationTable decodes exploration.bss once and memoizes it. buildWorld goes on
// to enrich these records in place (positions, links, names, territory); the item
// stage, which runs first, only reads their keys and Main flags.
func (b *Builder) explorationTable() ([]model.WorldNode, error) {
	if b.nodesDecoded != nil {
		return b.nodesDecoded, nil
	}
	data, err := b.src.Read("exploration.bss")
	if err != nil {
		return nil, err
	}
	nodes, err := tables.DecodeExplorationNodes(data)
	if err != nil {
		return nil, err
	}
	b.nodesDecoded = nodes

	return nodes, nil
}

// waypointTable decodes the node graph (mapdata_realexplore2.bwp) once and memoizes
// it, shared by buildWorld and the item stage's gather-node resolution.
func (b *Builder) waypointTable() (map[uint32]tables.WorldWaypoint, error) {
	if b.waypointsDecoded != nil {
		return b.waypointsDecoded, nil
	}
	data, err := b.src.Read("mapdata_realexplore2.bwp")
	if err != nil {
		return nil, err
	}
	waypoints, err := tables.DecodeWorldWaypoints(data)
	if err != nil {
		return nil, err
	}
	b.waypointsDecoded = waypoints

	return waypoints, nil
}

// npcTable decodes npcsimply.bss once and memoizes it, so the recipe stage's
// Korean-name lookup and the NPC stage share a single decode. The returned names
// are the client's raw (Korean) strings until buildNpcs applies the loc-6 English
// override; the recipe stage, which runs first, relies on the Korean names.
func (b *Builder) npcTable() ([]model.NPC, error) {
	if b.npcsDecoded != nil {
		return b.npcsDecoded, nil
	}
	data, err := b.src.Read("npcsimply.bss")
	if err != nil {
		return nil, err
	}
	npcs, err := tables.DecodeNPCs(data)
	if err != nil {
		return nil, err
	}
	b.npcsDecoded = npcs
	return npcs, nil
}

// buildNpcs decodes NPCs, attaches each NPC's spawn locations from the world
// regions (built by buildWorld), and registers npcs.json.
func (b *Builder) buildNpcs() error {
	npcs, err := b.npcTable()
	if err != nil {
		return err
	}
	spawnTypeData, err := b.src.Read("characterspawntype.dbss")
	if err != nil {
		return err
	}
	spawnTypeIndex, err := b.src.Read("characterspawntypeoffset.dbss")
	if err != nil {
		return err
	}
	spawnTypes, err := tables.DecodeCharacterSpawnTypes(spawnTypeIndex, spawnTypeData)
	if err != nil {
		return err
	}
	functionIndex, err := b.src.Read("characterfunctionoffset.dbss")
	if err != nil {
		return err
	}
	functionData, err := b.src.Read("characterfunction.dbss")
	if err != nil {
		return err
	}
	itemServices, err := tables.DecodeCharacterItemServices(functionIndex, functionData)
	if err != nil {
		return err
	}
	itemServiceIDs := make(map[uint32]bool, len(itemServices))
	for id := range itemServices {
		itemServiceIDs[id] = true
	}
	// npcsimply stores Korean names inline and omits a few map-role characters.
	// Join the English names and role flags, then add localized role-bearing
	// characters so every node-manager and town-service reference can resolve.
	var added int
	npcs, added = augmentNPCs(npcs, spawnTypes, itemServiceIDs, b.gs.EntityNames, b.gs.EntityTitles)
	roleNPCs := 0
	for i := range npcs {
		if len(npcs[i].SpawnTypes) > 0 {
			roleNPCs++
		}
	}

	// attach spawn locations to each NPC by character id; the region's place name
	// (loc table 17, keyed by region) gives the town/area.
	byChar := map[uint32][]model.NPCSpawn{}
	for i := range b.regions {
		r := &b.regions[i]
		key := uint32(r.Key)
		name := b.gs.Topography[key]
		for _, s := range r.Spawns {
			byChar[s.Key] = append(byChar[s.Key], model.NPCSpawn{
				Region:      model.WorldRegionRef(key),
				RegionKey:   key,
				RegionName:  name,
				Pos:         s.Pos,
				DialogIndex: s.DialogIndex,
			})
		}
	}
	located := 0
	attachedServices := 0
	conditionedServices := 0
	for i := range npcs {
		if service, ok := itemServices[npcs[i].ID]; ok {
			npcs[i].ItemService = &model.NPCItemService{
				Name:         service.SourceName,
				ConditionDSL: service.ConditionDSL,
				UnknownType:  service.Unknown0,
				UnknownKey:   service.UnknownKey,
			}
			attachedServices++
			if service.ConditionDSL != "" {
				conditionedServices++
			}
		}
		if sp := byChar[npcs[i].ID]; sp != nil {
			npcs[i].Spawns = sp
			located++
		}
	}
	b.logf(fmt.Sprintf(
		"npc item services: %d/%d attached, %d with access conditions",
		attachedServices, len(itemServices), conditionedServices,
	))
	missingManagers := missingNodeManagerSpawns(b.nodesDecoded, npcs)
	if len(missingManagers) > 0 {
		return fmt.Errorf(
			"%d/%d node-manager templates have no placement for language %q and region %q: %v",
			len(missingManagers), managerCount(b.nodesDecoded), b.lang, b.region, missingManagers,
		)
	}
	b.logf(fmt.Sprintf("node managers: all %d owner templates have placements", managerCount(b.nodesDecoded)))
	for _, manager := range distantNodeManagers(b.nodesDecoded, npcs, 10*12800) {
		b.logf(fmt.Sprintf(
			"WARNING node manager %d is %.0f units from owner node %d",
			manager.characterKey, manager.distance, manager.nodeKey,
		))
	}
	np, err := b.addJSON("npcs.json", npcs)
	if err != nil {
		return err
	}
	b.logf(fmt.Sprintf("npcs: %d (%d added from loc roles/services, %d located, %d with map roles) -> %s", len(npcs), added, located, roleNPCs, np))

	b.npcs = npcs

	return nil
}

func augmentNPCs(
	npcs []model.NPC,
	spawnTypes map[uint32]model.NPCSpawnTypes,
	itemServiceIDs map[uint32]bool,
	names map[uint32]string,
	titles map[uint32]string,
) ([]model.NPC, int) {
	existing := make(map[uint32]bool, len(npcs))
	for i := range npcs {
		existing[npcs[i].ID] = true
		if name := names[npcs[i].ID]; name != "" {
			npcs[i].Name = name
		}
		if title := titles[npcs[i].ID]; title != "" {
			npcs[i].Title = title
		}
		npcs[i].SpawnTypes = spawnTypes[npcs[i].ID]
	}

	candidates := make(map[uint32]bool, len(spawnTypes)+len(itemServiceIDs))
	for id, roles := range spawnTypes {
		if roles.HasMapRole() {
			candidates[id] = true
		}
	}
	for id := range itemServiceIDs {
		candidates[id] = true
	}

	ids := make([]int, 0, len(candidates))
	for id := range candidates {
		if !existing[id] && (names[id] != "" || itemServiceIDs[id]) {
			ids = append(ids, int(id))
		}
	}
	sort.Ints(ids)
	for _, rawID := range ids {
		id := uint32(rawID)
		npcs = append(npcs, model.NPC{
			BaseFor:    models.NewBaseFor[model.NPC](id),
			ID:         id,
			Name:       names[id],
			Title:      titles[id],
			SpawnTypes: spawnTypes[id],
		})
	}

	return npcs, len(ids)
}

func managerCount(nodes []model.WorldNode) int {
	count := 0
	for i := range nodes {
		if nodes[i].Manager != nil {
			count++
		}
	}
	return count
}

func missingNodeManagerSpawns(nodes []model.WorldNode, npcs []model.NPC) []uint32 {
	placed := make(map[uint32]bool, len(npcs))
	for i := range npcs {
		if len(npcs[i].Spawns) > 0 {
			placed[npcs[i].ID] = true
		}
	}
	missing := make([]uint32, 0)
	for i := range nodes {
		if nodes[i].Manager == nil {
			continue
		}
		characterKey := nodes[i].Manager.ID()
		if !placed[characterKey] {
			missing = append(missing, characterKey)
		}
	}
	slices.Sort(missing)
	return slices.Compact(missing)
}

type distantNodeManager struct {
	nodeKey      uint32
	characterKey uint32
	distance     float64
}

func distantNodeManagers(nodes []model.WorldNode, npcs []model.NPC, threshold float64) []distantNodeManager {
	spawnsByCharacter := make(map[uint32][]model.NPCSpawn, len(npcs))
	for i := range npcs {
		spawnsByCharacter[npcs[i].ID] = npcs[i].Spawns
	}

	var distant []distantNodeManager
	for i := range nodes {
		if nodes[i].Manager == nil {
			continue
		}
		characterKey := nodes[i].Manager.ID()
		nearest := math.Inf(1)
		for _, spawn := range spawnsByCharacter[characterKey] {
			distance := math.Hypot(
				nodes[i].Position[0]-spawn.Pos[0],
				nodes[i].Position[2]-spawn.Pos[2],
			)
			nearest = min(nearest, distance)
		}
		if nearest > threshold {
			distant = append(distant, distantNodeManager{
				nodeKey:      uint32(nodes[i].Key),
				characterKey: characterKey,
				distance:     nearest,
			})
		}
	}

	slices.SortFunc(distant, func(a, b distantNodeManager) int {
		return cmp.Compare(a.nodeKey, b.nodeKey)
	})
	return distant
}
