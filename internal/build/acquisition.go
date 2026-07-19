package build

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

// itemAcquisition is what an item's info XMLs say about getting it, in the client's
// own prose: the <shop> vendor names, the <collect> gather sources, and the
// <node region="…"> strings. scanItemInfo collects it; attachAcquisition resolves it.
type itemAcquisition struct{ vendors, gather, nodes []string }

// attachAcquisition resolves the collected acquisition names to refs on each item:
// vendor names to their npcsimply NPCs and node-region names to worldmap nodes.
// Gather sources stay as names — they're creatures and plants, not entities we model.
func (b *Builder) attachAcquisition(info map[uint32]*itemAcquisition) {
	npcsByName := b.npcIndexByName()
	gatherNodes, err := b.gatherNodeIndex()
	if err != nil {
		b.logf(fmt.Sprintf("gather nodes: node tables unavailable (%v) — items keep no node refs", err))
	}

	unresolvedVendors, unresolvedNodes := map[string]bool{}, map[string]bool{}
	for id, m := range info {
		it := b.items[id]
		if it == nil {
			continue
		}
		sort.Strings(m.vendors)
		sort.Strings(m.gather)
		sort.Strings(m.nodes)

		var npcIDs []uint32
		for _, name := range m.vendors {
			ids := npcsByName[strings.ToLower(name)]
			if len(ids) == 0 {
				unresolvedVendors[name] = true
				it.UnresolvedVendors = append(it.UnresolvedVendors, name)

				continue
			}
			npcIDs = append(npcIDs, ids...)
		}

		var nodeKeys []uint32
		for _, name := range m.nodes {
			key, ok := gatherNodes[name]
			if !ok {
				unresolvedNodes[name] = true
				it.UnresolvedGatherNodes = append(it.UnresolvedGatherNodes, name)

				continue
			}
			nodeKeys = append(nodeKeys, key)
		}

		it.Vendors = model.NpcRefList(npcIDs...)
		it.GatheredFrom = m.gather
		it.GatherNodes = model.WorldNodeRefList(nodeKeys...)
	}
	b.logf(fmt.Sprintf(
		"acquisition: %d vendor names with no npcsimply record, %d gather-node names unresolved",
		len(unresolvedVendors), len(unresolvedNodes),
	))
}

// attachItemRentals joins contribution-point rental dialogue actions onto items.
func (b *Builder) attachItemRentals() error {
	offsetData, err := b.src.Read("detail_dialogoffset.dbss")
	if err != nil {
		return err
	}
	data, err := b.src.Read("detail_dialog.dbss")
	if err != nil {
		return err
	}
	rows, err := tables.DecodeItemRentals(offsetData, data)
	if err != nil {
		return err
	}

	attached := 0
	missing := 0
	for _, row := range rows {
		item := b.items[row.ItemKey]
		if item == nil {
			missing++
			continue
		}
		item.RentalOffers = append(item.RentalOffers, model.ItemRentalOffer{
			Vendor:       model.NPCRef(row.CharacterKey),
			DialogIndex:  int(row.DialogIndex),
			ConditionDSL: row.ConditionDSL,
			Count:        int(row.Count),
			PointType:    int(row.PointType),
			PointCost:    int(row.PointCost),
			ItemSubKey:   row.ItemSubKey,
			Unknown0:     row.Unknown0,
		})
		attached++
	}
	b.logf(fmt.Sprintf("item rentals: %d offers attached, %d item records missing", attached, missing))

	return nil
}

// npcIndexByName maps a lowercased localized entity name to the ids of the NPCs
// carrying it — one vendor name is often several placed NPCs, one per town. Built
// from the loc names (in the build's language) rather than the npcsimply records,
// whose names are still the client's Korean strings at this stage.
func (b *Builder) npcIndexByName() map[string][]uint32 {
	npcs, err := b.npcTable()
	if err != nil {
		return nil
	}
	isNpc := make(map[uint32]bool, len(npcs))
	for _, n := range npcs {
		isNpc[n.ID] = true
	}

	idsByName := map[string][]uint32{}
	for id, e := range b.gs.Entities {
		if e.Name == "" || !isNpc[id] {
			continue
		}
		key := strings.ToLower(e.Name)
		idsByName[key] = append(idsByName[key], id)
	}
	for _, ids := range idsByName {
		slices.Sort(ids)
	}

	return idsByName
}

// gatherNodeIndex maps the node-region strings the item XMLs use — "<main node> -
// <sub-node>", e.g. "Ahto Farm - Cotton Farming" — to the sub-node's key, by composing
// each main node's name with those of its sub-nodes in the waypoint graph.
func (b *Builder) gatherNodeIndex() (map[string]uint32, error) {
	nodes, err := b.explorationTable()
	if err != nil {
		return nil, err
	}
	waypoints, err := b.waypointTable()
	if err != nil {
		return nil, err
	}

	main := make(map[uint32]bool, len(nodes))
	for i := range nodes {
		main[uint32(nodes[i].Key)] = nodes[i].Main
	}

	index := map[string]uint32{}
	for i := range nodes {
		if !nodes[i].Main {
			continue
		}
		parent := b.gs.NodeNames[uint32(nodes[i].Key)]
		if parent == "" {
			continue
		}
		for _, key := range waypoints[uint32(nodes[i].Key)].Links {
			child := b.gs.NodeNames[key]
			if child == "" || main[key] {
				continue
			}
			index[parent+" - "+child] = key
		}
	}

	return index, nil
}
