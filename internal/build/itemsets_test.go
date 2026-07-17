package build

import (
	"testing"

	"github.com/idevelopthings/bdo-data-extractor/internal/loc"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

func TestLocalizeItemSets(t *testing.T) {
	sets := []model.ItemSet{{
		SkillNo: 58080,
		Bonuses: []model.ItemSetBonus{{
			Pieces:           3,
			Apply:            1,
			GroupTitle:       "source group",
			DescriptionTitle: "source tier",
			Description:      "source description",
		}},
	}}
	strings := loc.Table{
		58080: {
			1:          "All AP +12",
			0x01000001: "3 Parts",
			0x02000001: "Deboreka Accessory Effect",
		},
	}

	if got := localizeItemSets(sets, strings); got != 3 {
		t.Fatalf("localized fields = %d, want 3", got)
	}
	bonus := sets[0].Bonuses[0]
	if bonus.Description != "All AP +12" || bonus.DescriptionTitle != "3 Parts" || bonus.GroupTitle != "Deboreka Accessory Effect" {
		t.Fatalf("localized bonus = %#v", bonus)
	}
}

func TestLocalizeItemSetsKeepsSourceFallback(t *testing.T) {
	sets := []model.ItemSet{{
		SkillNo: 50068,
		Bonuses: []model.ItemSetBonus{{Description: "source"}},
	}}

	if got := localizeItemSets(sets, nil); got != 0 {
		t.Fatalf("localized fields = %d, want 0", got)
	}
	if sets[0].Bonuses[0].Description != "source" {
		t.Fatalf("description = %q, want source fallback", sets[0].Bonuses[0].Description)
	}
}

func TestItemSetForMarker(t *testing.T) {
	tests := []struct {
		marker string
		want   uint32
	}{
		{"BLACKSTAR_NO_2_SET_EFFECT", 52494},
		{"DEBOREKA_NO_5_SET_EFFECT", 58080},
		{"TUNGRAD_NO_3_SET_EFFECT", 58454},
		{"ANCIENT_NO_4_SET_EFFECT", 57337},
		{"EDANA_NO_2_SET_EFFECT", 57337},
		{"GBEAR_1_SET_EFFECT", 47639},
		{"SET_DECORATE_Training", 57482},
		{"NO_3_SET_EFFECT", 0},
	}
	for _, test := range tests {
		t.Run(test.marker, func(t *testing.T) {
			if got := itemSetForMarker(test.marker); got != test.want {
				t.Fatalf("itemSetForMarker(%q) = %d, want %d", test.marker, got, test.want)
			}
		})
	}
}
