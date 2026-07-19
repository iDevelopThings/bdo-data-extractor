package model

import "testing"

func TestItemTypeTooltipTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		itemType ItemType
		forTrade bool
		want     string
	}{
		{name: "consumable", itemType: ItemTypeSkill, want: "Consumable"},
		{name: "consumable trade item", itemType: ItemTypeSkill, forTrade: true, want: "Trade Item"},
		{name: "crafting material", itemType: ItemTypeMaterial, want: "Crafting Material"},
		{name: "crafting trade item", itemType: ItemTypeMaterial, forTrade: true, want: "Crafting Material/Trade Item"},
		{name: "general trade item", itemType: ItemTypeNormal, forTrade: true, want: "General/Trade Item"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.itemType.TooltipTitle(tt.forTrade); got != tt.want {
				t.Fatalf("TooltipTitle(%v) = %q, want %q", tt.forTrade, got, tt.want)
			}
		})
	}
}
