package model

import "testing"

func TestBuffStackingCategories(t *testing.T) {
	t.Parallel()

	var categories BuffStackingCategories
	categories.Add(BuffStackingCategoryPerfume)
	categories.Add(BuffStackingCategoryPerfume)
	categories.Add(0)
	if len(categories) != 1 || !categories.Has(BuffStackingCategoryPerfume) {
		t.Fatalf("categories = %v", categories)
	}
	if categories.Has(BuffStackingCategoryFood) {
		t.Fatal("categories unexpectedly contains food")
	}
}

func TestBuffStackingCategoriesSortKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		categories BuffStackingCategories
		want       int
	}{
		{name: "draught", categories: BuffStackingCategories{BuffStackingCategoryElixir, BuffStackingCategoryDraughtResetControl}, want: 6},
		{name: "perfume", categories: BuffStackingCategories{BuffStackingCategoryPerfume}, want: 5},
		{name: "cron meal", categories: BuffStackingCategories{BuffStackingCategoryFood, BuffStackingCategoryCronMealExtra}, want: 4},
		{name: "food", categories: BuffStackingCategories{BuffStackingCategoryFood}, want: 3},
		{name: "elixir", categories: BuffStackingCategories{BuffStackingCategoryElixir}, want: 2},
		{name: "whale tendon elixir", categories: BuffStackingCategories{BuffStackingCategoryWhaleTendonElixir}, want: 1},
		{name: "uncategorized", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.categories.SortKey(); got != tt.want {
				t.Fatalf("SortKey() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestFormatEffectFunctionsPreservesSetMarker(t *testing.T) {
	groups := FormatEffectFunctions([]EffectDsl{
		{Func: "DEBOREKA_NO_3_SET_EFFECT"},
		{Func: "ALL_AP_UP", Args: []float64{12}},
	}, true, "")

	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	if groups[0].Marker != "DEBOREKA_NO_3_SET_EFFECT" || groups[0].Pieces != 3 {
		t.Fatalf("group marker = %q pieces = %d", groups[0].Marker, groups[0].Pieces)
	}
}

func TestFormatEffectFunctionsRecognizesTrainingCostumeSet(t *testing.T) {
	groups := FormatEffectFunctions([]EffectDsl{
		{Func: "SET_DECORATE_Training"},
		{Func: "MOUN_EXP_POINT_ADD", Args: []float64{15}},
	}, true, "")

	if len(groups) != 1 || groups[0].Marker != "SET_DECORATE_Training" {
		t.Fatalf("groups = %#v", groups)
	}
}

func TestEffectCurveFieldsOnlyApplyToArglessDirectives(t *testing.T) {
	argless, ok := EffectFuncToStatMod(EffectDsl{Func: "ALL_EVA_INCRE"})
	if !ok || len(argless.CurveFields) != 2 || argless.Value != 0 {
		t.Fatalf("argless directive = %#v", argless)
	}

	valued, ok := EffectFuncToStatMod(EffectDsl{Func: "ALL_EVA_INCRE", Args: []float64{8}})
	if !ok || valued.Value != 8 || valued.Op != "+" || len(valued.CurveFields) != 0 {
		t.Fatalf("valued effect = %#v", valued)
	}
	if valued.EffectDsl == nil || len(valued.EffectDsl.Args) != 1 || valued.EffectDsl.Args[0] != 8 {
		t.Fatalf("raw DSL = %#v", valued.EffectDsl)
	}
	if argless.StatID != "" || valued.StatID != "" {
		t.Fatalf("curve directives must remain context-routed, got stat IDs %q/%q", argless.StatID, valued.StatID)
	}
}

func TestContextualAPFunctionsHaveNoGlobalStatRoute(t *testing.T) {
	for _, fn := range []string{"ALL_AP_INCRE", "ALL_AP_INCRE_VALUE", "ALL_AP_UP"} {
		mod, ok := EffectFuncToStatMod(EffectDsl{Func: fn, Args: []float64{8}})
		if !ok || mod.StatID != "" {
			t.Errorf("%s resolved to global stat %q", fn, mod.StatID)
		}
	}
}

func TestBossResistanceMarkersExpandBundledStats(t *testing.T) {
	for _, fn := range []string{"KU_ALL_REG_ADD", "NU_ALL_REG_ADD"} {
		t.Run(fn, func(t *testing.T) {
			mods, ok := EffectFuncToStatMods(EffectDsl{Func: fn})
			if !ok || len(mods) != 2 {
				t.Fatalf("mods = %#v", mods)
			}
			if mods[0].Stat != "All Resistance" || mods[0].Value != 10 || mods[0].Unit != "%" {
				t.Fatalf("resistance = %#v", mods[0])
			}
			if mods[0].StatID != StatIdAllResistance || mods[1].StatID != StatIdSpecialAttackDamage {
				t.Fatalf("stat IDs = %q/%q", mods[0].StatID, mods[1].StatID)
			}
			if mods[1].Func != "ALL_SPECIAL_ATT_DAM_ADD" || mods[1].Stat != "All Special Attack Extra Damage" || mods[1].Value != 10 || mods[1].Unit != "%" || mods[1].DerivedFrom != fn {
				t.Fatalf("special attack = %#v", mods[1])
			}
		})
	}
}

func TestBossResistanceMarkerKeepsExplicitArgument(t *testing.T) {
	mods, ok := EffectFuncToStatMods(EffectDsl{Func: "KU_ALL_REG_ADD", Args: []float64{7}})
	if !ok || len(mods) != 1 || mods[0].Value != 7 {
		t.Fatalf("mods = %#v", mods)
	}
}
