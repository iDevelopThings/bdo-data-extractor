package build

import (
	"reflect"
	"testing"

	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

func TestAugmentNPCs(t *testing.T) {
	npcs := []model.NPC{{ID: 20, Name: "raw"}}
	roles := map[uint32]model.NPCSpawnTypes{
		10: {model.NPCSpawnTypeExplorer},
		20: {model.NPCSpawnTypeWarehouse},
		30: {model.NPCSpawnTypeExplorer},
		40: {model.NPCSpawnTypeNormal},
	}
	names := map[uint32]string{
		10: "First Manager",
		20: "Existing Keeper",
		40: "No Role",
	}
	titles := map[uint32]string{
		10: "<Manager>",
		20: "<Warehouse Keeper>",
		40: "<Normal NPC>",
	}
	itemServices := map[uint32]bool{50: true, 60: true}
	names[50] = "Service NPC"

	got, added := augmentNPCs(npcs, roles, itemServices, names, titles)
	if added != 3 {
		t.Fatalf("added = %d, want 3", added)
	}
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4", len(got))
	}
	if got[0].Name != "Existing Keeper" || !reflect.DeepEqual(got[0].SpawnTypes, roles[20]) {
		t.Fatalf("existing NPC = %+v", got[0])
	}
	if got[1].ID != 10 || got[1].Name != "First Manager" || !reflect.DeepEqual(got[1].SpawnTypes, roles[10]) {
		t.Fatalf("added NPC = %+v", got[1])
	}
	if got[2].ID != 50 || got[2].Name != "Service NPC" {
		t.Fatalf("service NPC = %+v", got[2])
	}
	if got[3].ID != 60 || got[3].Name != "" {
		t.Fatalf("unnamed service NPC = %+v", got[3])
	}
}

func TestNormalizeNodeManagers(t *testing.T) {
	nodes := []model.WorldNode{
		{Key: 10, Manager: model.NPCRef(100)},
		{Key: 20, Manager: model.NPCRef(100)},
		{Key: 30, Manager: model.NPCRef(200)},
	}

	owners, affiliates, err := normalizeNodeManagers(nodes, map[uint32]uint32{100: 20, 200: 30})
	if err != nil {
		t.Fatalf("normalizeNodeManagers: %v", err)
	}
	if owners != 2 || affiliates != 1 {
		t.Fatalf("counts = %d owners/%d affiliates, want 2/1", owners, affiliates)
	}
	if nodes[0].Manager != nil || nodes[0].ManagerNode == nil || nodes[0].ManagerNode.ID() != 20 {
		t.Fatalf("affiliate = %+v", nodes[0])
	}
	if nodes[1].Manager == nil || nodes[1].Manager.ID() != 100 || nodes[1].ManagerNode != nil {
		t.Fatalf("owner = %+v", nodes[1])
	}
}

func TestNodeManagerFamiliesRejectsPseudoNodeFamily(t *testing.T) {
	nodes := []model.WorldNode{
		{Key: 858, Kind: model.WorldNodeKindNormal, Manager: model.NPCRef(40001)},
		{Key: 961, Kind: model.WorldNodeKindNormal, Manager: model.NPCRef(40001)},
		{Key: 10, Kind: model.WorldNodeKindNormal, Main: true, Manager: model.NPCRef(100)},
		{Key: 20, Kind: model.WorldNodeKindCollect, Manager: model.NPCRef(100)},
		{Key: 1826, Kind: model.WorldNodeKindFarm, Manager: model.NPCRef(200)},
	}

	got := nodeManagerFamilies(nodes)
	want := map[uint32][]uint32{
		100: {10, 20},
		200: {1826},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("families = %v, want %v", got, want)
	}
	if nodes[0].Manager != nil || nodes[1].Manager != nil {
		t.Fatalf("pseudo-node manager family was retained: %+v", nodes[:2])
	}
}

func TestOverlayRegionSpawnsReplacesWholeRegion(t *testing.T) {
	base := map[uint32][]model.Spawn{
		1: {{Key: 10}, {Key: 20}},
		2: {{Key: 30}},
	}
	overlayRegionSpawns(base, map[uint32][]model.Spawn{
		1: {{Key: 40}},
		2: nil,
		3: {{Key: 50}},
	})
	want := map[uint32][]model.Spawn{
		1: {{Key: 40}},
		2: nil,
		3: {{Key: 50}},
	}
	if !reflect.DeepEqual(base, want) {
		t.Fatalf("regions = %v, want %v", base, want)
	}
}

func TestMissingNodeManagerSpawns(t *testing.T) {
	nodes := []model.WorldNode{
		{Key: 10, Manager: model.NPCRef(100)},
		{Key: 20, Manager: model.NPCRef(200)},
		{Key: 30, ManagerNode: model.WorldNodeRef(10)},
	}
	npcs := []model.NPC{
		{ID: 100, Spawns: []model.NPCSpawn{{Pos: [3]float64{1, 2, 3}}}},
		{ID: 200},
	}

	got := missingNodeManagerSpawns(nodes, npcs)
	want := []uint32{200}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("missing managers = %v, want %v", got, want)
	}
}

func TestDistantNodeManagers(t *testing.T) {
	nodes := []model.WorldNode{
		{Key: 10, Position: [3]float64{0, 0, 0}, Manager: model.NPCRef(100)},
		{Key: 20, Position: [3]float64{200, 0, 0}, Manager: model.NPCRef(200)},
	}
	npcs := []model.NPC{
		{ID: 100, Spawns: []model.NPCSpawn{{Pos: [3]float64{50, 0, 0}}}},
		{ID: 200, Spawns: []model.NPCSpawn{{Pos: [3]float64{0, 0, 0}}}},
	}

	got := distantNodeManagers(nodes, npcs, 100)
	want := []distantNodeManager{{nodeKey: 20, characterKey: 200, distance: 200}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("distant managers = %+v, want %+v", got, want)
	}
}
