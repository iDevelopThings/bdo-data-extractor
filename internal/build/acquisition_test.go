package build

import (
	"reflect"
	"testing"
	"time"

	"github.com/idevelopthings/bdo-data-extractor/internal/loc"
	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

func TestAttachAcquisitionPreservesUnresolvedNames(t *testing.T) {
	item := &model.Item{}
	b := &Builder{
		t0:    time.Now(),
		items: map[uint32]*model.Item{1: item},
		gs: &loc.GameStrings{
			Entities: map[uint32]loc.EntityText{100: {Name: "Known Vendor"}},
			NodeNames: map[uint32]string{
				10: "Known Farm",
				11: "Cotton Farming",
			},
		},
		npcsDecoded: []model.NPC{{ID: 100}},
		nodesDecoded: []model.WorldNode{
			{Key: 10, Main: true},
			{Key: 11},
		},
		waypointsDecoded: map[uint32]tables.WorldWaypoint{
			10: {Links: []uint32{11}},
		},
	}

	b.attachAcquisition(map[uint32]*itemAcquisition{
		1: {
			vendors: []string{"Unknown Vendor", "Known Vendor"},
			gather:  []string{"Wild Flax"},
			nodes:   []string{"Unknown Farm - Unknown Product", "Known Farm - Cotton Farming"},
		},
	})

	if item.Vendors == nil || item.Vendors.Len() != 1 || item.Vendors.URNs[0].Uint32Unsafe() != 100 {
		t.Fatalf("vendors = %+v", item.Vendors)
	}
	if want := []string{"Unknown Vendor"}; !reflect.DeepEqual(item.UnresolvedVendors, want) {
		t.Fatalf("unresolved vendors = %v, want %v", item.UnresolvedVendors, want)
	}
	if want := []string{"Wild Flax"}; !reflect.DeepEqual(item.GatheredFrom, want) {
		t.Fatalf("gathered from = %v, want %v", item.GatheredFrom, want)
	}
	if item.GatherNodes == nil || item.GatherNodes.Len() != 1 || item.GatherNodes.URNs[0].Uint32Unsafe() != 11 {
		t.Fatalf("gather nodes = %+v", item.GatherNodes)
	}
	if want := []string{"Unknown Farm - Unknown Product"}; !reflect.DeepEqual(item.UnresolvedGatherNodes, want) {
		t.Fatalf("unresolved gather nodes = %v, want %v", item.UnresolvedGatherNodes, want)
	}
}
