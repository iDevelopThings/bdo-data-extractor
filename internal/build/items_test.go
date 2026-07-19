package build

import (
	"encoding/binary"
	"slices"
	"testing"

	"github.com/idevelopthings/bdo-data-extractor/internal/loc"
	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

func TestMergeItemSkillEffects(t *testing.T) {
	t.Parallel()

	got := mergeItemSkillEffects([2]uint32{101, 202}, map[uint32]tables.SkillEffect{
		101: {CooldownMs: 5_000, Buffs: []uint16{10, 20}},
		202: {CooldownMs: 10_000, Buffs: []uint16{20, 30}},
	})
	if got.CooldownMs != 10_000 {
		t.Fatalf("cooldown = %d, want 10000", got.CooldownMs)
	}
	if want := []uint16{10, 20, 30}; !slices.Equal(got.Buffs, want) {
		t.Fatalf("buffs = %v, want %v", got.Buffs, want)
	}
}

func TestFillQuestAddsClientConditions(t *testing.T) {
	t.Parallel()

	b := &Builder{
		gs: &loc.GameStrings{Quests: map[uint32]map[uint32]loc.QuestText{
			895: {1: {Name: "Shakatu Merchants' Archive"}},
		}},
		questConditions: map[uint32]tables.QuestConditionRow{
			1<<16 | 895: {AcceptDSL: "ClearQuest(4501,3);", CompleteDSL: "meet(50493,1);"},
		},
	}
	quest := model.QuestRef{ID: "895-1"}
	b.fillQuest(&quest)
	if quest.Name != "Shakatu Merchants' Archive" || quest.Conditions == nil {
		t.Fatalf("quest = %+v", quest)
	}
	if quest.Conditions.AcceptDSL != "ClearQuest(4501,3);" || quest.Conditions.CompleteDSL != "meet(50493,1);" {
		t.Fatalf("conditions = %+v", quest.Conditions)
	}
}

func TestBuildEffectsPreservesBuffStackingRules(t *testing.T) {
	t.Parallel()

	stat := tables.Buff{Index: 10, Module: 39, Group: 5012, StackingCategory: 2, DurationMs: 900_000}
	binary.LittleEndian.PutUint32(stat.EffectData[7:], 3)
	binary.LittleEndian.PutUint32(stat.EffectData[15:], 30)
	reset := tables.Buff{Index: 20, Module: 58, Group: 8028, StackingCategory: 26}
	b := &Builder{gs: &loc.GameStrings{BuffNames: map[uint32]string{}}}

	effects := b.buildEffects(map[uint16]tables.Buff{10: stat, 20: reset}, tables.SkillEffect{
		CooldownMs: 10_000,
		Buffs:      []uint16{10, 20},
	})
	if effects == nil || len(effects.Stats.Stats) != 1 {
		t.Fatalf("buildEffects() = %#v", effects)
	}
	got := effects.Stats.Stats[0]
	if got.StatID != model.StatIdHiddenAp || got.BuffGroup != 5012 || got.BuffCategory != model.BuffStackingCategoryElixir {
		t.Fatalf("stat metadata = %#v", got)
	}
	if !slices.Equal(effects.BuffCategories, model.BuffStackingCategories{
		model.BuffStackingCategoryElixir,
		model.BuffStackingCategoryDraughtResetControl,
	}) {
		t.Fatalf("categories = %v", effects.BuffCategories)
	}
	if !slices.Equal(effects.ClearsBuffCategories, []model.BuffStackingCategory{model.BuffStackingCategoryElixir}) {
		t.Fatalf("clears = %v", effects.ClearsBuffCategories)
	}
}
