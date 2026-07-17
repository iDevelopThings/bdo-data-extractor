package model

import "testing"

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
