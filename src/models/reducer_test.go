package models_test

import (
	"testing"

	"github.com/idevelopthings/bdo-data-extractor/src/models"
)

func TestReducer_ResultNotReadyBeforeBuild(t *testing.T) {
	reg := models.NewRegistry()
	s := models.NewStore[testItem](0, nil)
	if err := s.Add(itemURN("1"), &testItem{ID: "1"}); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	r := models.NewReducer(s, 0, func(acc int, it *testItem) int { return acc + 1 })
	models.RegisterStoreIn(reg, s)

	if _, ok := r.Result(); ok {
		t.Fatalf("Result() ok = true before Build() ran, want false")
	}

	if err := reg.Build(); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	got, ok := r.Result()
	if !ok {
		t.Fatalf("Result() ok = false after Build() ran, want true")
	}
	if got != 1 {
		t.Errorf("Result() = %d, want 1", got)
	}
}

func TestReducer_StepCalledExactlyOncePerItem(t *testing.T) {
	reg := models.NewRegistry()
	s := models.NewStore[testItem](0, nil)
	for _, id := range []string{"1", "2", "3"} {
		if err := s.Add(itemURN(id), &testItem{ID: id}); err != nil {
			t.Fatalf("Add(%s) returned error: %v", id, err)
		}
	}

	var stepCalls int
	r := models.NewReducer(s, 0, func(acc int, it *testItem) int {
		stepCalls++
		return acc + 1
	})
	models.RegisterStoreIn(reg, s)

	if err := reg.Build(); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if stepCalls != 3 {
		t.Fatalf("step called %d times for 3 items, want exactly 3 (one pass, no double-counting)", stepCalls)
	}

	got, _ := r.Result()
	if got != 3 {
		t.Errorf("Result() = %d, want 3", got)
	}
}

func TestReducer_SharesOnePassWithOtherHooksOnSameStore(t *testing.T) {
	// If Reducer added its own separate walk over items instead of
	// piggybacking on the store's existing hook pass, this test would
	// still pass by coincidence. What actually proves single-pass
	// sharing is call interleaving: the explicit hook and the reducer's
	// internal hook must alternate per item (h, reduce, h, reduce, ...),
	// not run as two back-to-back full sweeps (h, h, h, reduce, reduce,
	// reduce).
	reg := models.NewRegistry()
	s := models.NewStore[testItem](0, nil)
	for _, id := range []string{"1", "2", "3"} {
		if err := s.Add(itemURN(id), &testItem{ID: id}); err != nil {
			t.Fatalf("Add(%s) returned error: %v", id, err)
		}
	}

	var order []string
	s.AddHook(func(it *testItem) error {
		order = append(order, "hook:"+it.ID)
		return nil
	})
	models.NewReducer(s, 0, func(acc int, it *testItem) int {
		order = append(order, "reduce:"+it.ID)
		return acc + 1
	})

	models.RegisterStoreIn(reg, s)
	if err := reg.Build(); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	want := []string{"hook:1", "reduce:1", "hook:2", "reduce:2", "hook:3", "reduce:3"}
	if len(order) != len(want) {
		t.Fatalf("call order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("call order = %v, want %v (not interleaved — Reducer is doing its own separate pass)", order, want)
		}
	}
}

// categoryCount mirrors the two-level counter (category total + per-
// subcategory breakdown) from the original nested-loop updateCount.
type categoryCount struct {
	total         int
	bySubCategory map[string]int
}

type marketItem struct {
	Category    string
	SubCategory string
}

func TestReducer_CategoryCountsMatchOriginalNestedLoopSemantics(t *testing.T) {
	reg := models.NewRegistry()
	s := models.NewStore[marketItem](0, nil)

	items := []marketItem{
		{Category: "Weapons", SubCategory: "Sword"},
		{Category: "Weapons", SubCategory: "Sword"},
		{Category: "Weapons", SubCategory: "Bow"},
		{Category: "Weapons", SubCategory: "Dagger"}, // subcategory not in any known list
		{Category: "Armor", SubCategory: "Helmet"},
	}
	for i, it := range items {
		if err := s.Add(itemURN(string(rune('a'+i))), &marketItem{Category: it.Category, SubCategory: it.SubCategory}); err != nil {
			t.Fatalf("Add returned error: %v", err)
		}
	}

	counts := models.NewReducer(s, map[string]*categoryCount{}, func(acc map[string]*categoryCount, it *marketItem) map[string]*categoryCount {
		cc, ok := acc[it.Category]
		if !ok {
			cc = &categoryCount{bySubCategory: make(map[string]int)}
			acc[it.Category] = cc
		}
		cc.total++
		cc.bySubCategory[it.SubCategory]++
		return acc
	})

	models.RegisterStoreIn(reg, s)
	if err := reg.Build(); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	result, ok := counts.Result()
	if !ok {
		t.Fatalf("Result() ok = false after Build()")
	}

	weapons, ok := result["Weapons"]
	if !ok {
		t.Fatalf("no count entry for Weapons")
	}
	if weapons.total != 4 {
		t.Errorf("Weapons total = %d, want 4 (matches original: category count is independent of subcategory match)", weapons.total)
	}
	if weapons.bySubCategory["Sword"] != 2 {
		t.Errorf("Weapons/Sword = %d, want 2", weapons.bySubCategory["Sword"])
	}
	if weapons.bySubCategory["Bow"] != 1 {
		t.Errorf("Weapons/Bow = %d, want 1", weapons.bySubCategory["Bow"])
	}

	armor, ok := result["Armor"]
	if !ok {
		t.Fatalf("no count entry for Armor")
	}
	if armor.total != 1 || armor.bySubCategory["Helmet"] != 1 {
		t.Errorf("Armor = %+v, want total=1 Helmet=1", armor)
	}
}
