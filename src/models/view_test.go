package models_test

import (
	"sort"
	"testing"

	"github.com/idevelopthings/bdo-data-extractor/src/models"
)

func TestView_ComputesCorrectDerivedValue(t *testing.T) {
	s := models.NewStore[testItem](0, nil)
	names := []string{"Zed Sword", "Alpha Shield", "Mid Bow"}
	for i, name := range names {
		id := string(rune('1' + i))
		if err := s.Add(itemURN(id), &testItem{ID: id, Name: name}); err != nil {
			t.Fatalf("Add returned error: %v", err)
		}
	}

	byName := models.NewView(
		s, func(all []*testItem) []string {
			out := make([]string, len(all))
			for i, it := range all {
				out[i] = it.Name
			}
			sort.Strings(out)
			return out
		},
	)

	got := byName.Get()
	want := []string{"Alpha Shield", "Mid Bow", "Zed Sword"}
	if len(got) != len(want) {
		t.Fatalf("View.Get() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("View.Get()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestView_MemoizesAcrossMultipleGetCalls(t *testing.T) {
	s := models.NewStore[testItem](0, nil)
	if err := s.Add(itemURN("1"), &testItem{ID: "1", Name: "Sword"}); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	var buildCalls int
	v := models.NewView(
		s, func(all []*testItem) int {
			buildCalls++
			return len(all)
		},
	)

	for i := 0; i < 5; i++ {
		v.Get()
	}

	if buildCalls != 1 {
		t.Fatalf("build function called %d times across 5 Get() calls, want exactly 1", buildCalls)
	}
}

func TestView_DoesNotSeeItemsAddedAfterFirstGet(t *testing.T) {
	// Documents the memoization contract: a View is a snapshot as of its
	// first Get(), not a live recomputation. Store mutation is only
	// supposed to happen during initial load anyway, but this pins down
	// the actual behavior so it can't silently change later.
	s := models.NewStore[testItem](0, nil)
	if err := s.Add(itemURN("1"), &testItem{ID: "1"}); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	v := models.NewView(s, func(all []*testItem) int { return len(all) })

	if got := v.Get(); got != 1 {
		t.Fatalf("first Get() = %d, want 1", got)
	}

	if err := s.Add(itemURN("2"), &testItem{ID: "2"}); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	if got := v.Get(); got != 1 {
		t.Fatalf("second Get() = %d, want memoized 1 (View should not recompute)", got)
	}
}

func TestView_BuildReceivesStoreInsertionOrder(t *testing.T) {
	s := models.NewStore[testItem](0, nil)
	ids := []string{"3", "1", "2"}
	for _, id := range ids {
		if err := s.Add(itemURN(id), &testItem{ID: id}); err != nil {
			t.Fatalf("Add(%s) returned error: %v", id, err)
		}
	}

	v := models.NewView(
		s, func(all []*testItem) []string {
			out := make([]string, len(all))
			for i, it := range all {
				out[i] = it.ID
			}
			return out
		},
	)

	got := v.Get()
	for i, id := range ids {
		if got[i] != id {
			t.Errorf("View saw All()[%d] = %q, want %q (insertion order)", i, got[i], id)
		}
	}
}
