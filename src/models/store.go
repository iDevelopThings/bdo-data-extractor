package models

import (
	"fmt"
	"reflect"

	"github.com/idevelopthings/bdo-data-extractor/src/urn"
	"github.com/idevelopthings/bdo-data-extractor/src/utils"
)

// Hook is a per-item callback registered against a Store[T]. Every hook
// registered on a store runs, for every item in that store, during a
// Build() call — in registration order, over the store's insertion
// order. Hooks may call Resolve[Other] freely: Build() guarantees every
// registered store is fully populated (via Load) before any hook runs,
// so there's no load-ordering dependency between sources.
type Hook[T any] func(item *T) error

// Store is a read-mostly, URN-keyed index for one concrete entity type T
// (Item, Quest, Region, ...). It's populated once during catalog load
// (Add), optionally has Hooks registered against it for cross-
// referencing work, and is read-only once Build() has run. Add is not
// synchronized — build it single-threaded during Load, before
// RegisterStore.
type Store[T any] struct {
	byURN    map[urn.URN]*T
	ordered  []*T // insertion order; also the iteration base for hooks/views
	validate func(urn.URN) bool
	hooks    []Hook[T]
	built    bool // true once runHooks has completed for this store
}

// NewStore creates a Store with a size hint and an optional validate
// func that rejects URNs which don't belong to this store (e.g. wrong
// Domain/Kind). Pass nil to skip validation.
func NewStore[T any](capacityHint int, validate func(urn.URN) bool) *Store[T] {
	return &Store[T]{
		byURN:    make(map[urn.URN]*T, capacityHint),
		ordered:  make([]*T, 0, capacityHint),
		validate: validate,
	}
}

// Add inserts v under u, preserving first-seen insertion order for All().
// Re-adding an existing URN updates the map value in place but does not
// duplicate the ordered entry (and does not reorder it).
func (s *Store[T]) Add(u urn.URN, v *T) error {
	if s.validate != nil && !s.validate(u) {
		return fmt.Errorf("urn %s does not belong in this store (%T)", u, *new(T))
	}
	if _, exists := s.byURN[u]; !exists {
		s.ordered = append(s.ordered, v)
	}
	s.byURN[u] = v
	return nil
}

// Get looks up v by URN. Returns (nil, false) for an invalid/zero URN
// without probing the map.
func (s *Store[T]) Get(u urn.URN) (*T, bool) {
	if !u.IsValid() {
		return nil, false
	}
	v, ok := s.byURN[u]
	return v, ok
}
func (s *Store[T]) GetUnsafe(u urn.URN) *T {
	if !u.IsValid() {
		return nil
	}
	v, ok := s.byURN[u]
	if !ok {
		return nil
	}
	return v
}
func (s *Store[T]) Len() int {
	return len(s.ordered)
}

// All returns every item in insertion order. Views are built on top of
// this slice — treat the returned slice as read-only; clone before
// sorting or otherwise mutating in place.
func (s *Store[T]) All() []*T {
	return s.ordered
}

func (s *Store[T]) Each(fn func(it *T, shouldBreak *bool) error) error {
	shouldBreak := false
	for _, item := range s.ordered {
		if err := fn(item, &shouldBreak); err != nil {
			return err
		}
		if shouldBreak {
			break
		}
	}
	return nil
}

func (s *Store[T]) EachNoBreak(fn func(it *T) error) error {
	for _, item := range s.ordered {
		if err := fn(item); err != nil {
			return err
		}
	}
	return nil
}

// AddHook registers a per-item callback that runs during Build().
func (s *Store[T]) AddHook(h Hook[T]) {
	s.hooks = append(s.hooks, h)
}

// Built reports whether this store's hooks have run to completion (i.e.
// a Build() call that reached this store has finished). Reducer.Result()
// uses this to distinguish "not computed yet" from a genuine zero value.
func (s *Store[T]) Built() bool { return s.built }

func (s *Store[T]) runHooks() error {
	if len(s.hooks) == 0 {
		s.built = true
		return nil
	}

	timed := utils.Timed(fmt.Sprintf(
		"[STORE] %s: %d hooks over %d items", reflect.TypeFor[T]().String(), len(s.hooks), len(s.ordered),
	))
	defer timed()

	for _, item := range s.ordered {
		for _, h := range s.hooks {
			if err := h(item); err != nil {
				return err
			}
		}
	}
	s.built = true
	return nil
}
