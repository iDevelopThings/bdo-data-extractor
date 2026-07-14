package models

import (
	"log"
	"reflect"
	"sync"

	"github.com/idevelopthings/bdo-data-extractor/src/urn"
	"github.com/idevelopthings/bdo-data-extractor/src/utils"
)

// runner lets a Registry invoke hooks on a *Store[T] without the
// Registry itself needing to be generic over T.
type runner interface {
	runHooks() error
}

// Registry holds the type -> *Store[T] index and the ordered list of
// stores whose hooks run during Build().
//
// Production code normally uses the package-level Default registry via
// RegisterStore / Resolve / Build. Tests should construct their own via
// NewRegistry() for isolation from other tests and from Default.
type Registry struct {
	mu      sync.RWMutex
	byType  map[reflect.Type]any
	runners []runner
}

func NewRegistry() *Registry {
	return &Registry{byType: make(map[reflect.Type]any)}
}

// RegisterStoreIn registers s against r: as the resolver for T, and as a
// participant in r.Build(). Call once per concrete entity type, after
// that type's Store is fully populated by its Source's Load().
func RegisterStoreIn[T any](r *Registry, s *Store[T]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byType[reflect.TypeFor[T]()] = s
	r.runners = append(r.runners, s)
}

// Reset clears r back to empty: no registered stores, no build participants.
// Used to reload a dataset in place (e.g. after a re-extraction) so the next
// LoadAll+Build produces the same clean state as a fresh process, rather than
// stacking a second set of stores on top of the first.
func (r *Registry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byType = make(map[reflect.Type]any)
	r.runners = nil
}

// StoreForIn returns the *Store[T] registered against r, or nil if there is
// none. Resolve a batch of URNs through this once rather than paying the
// registry's map lookup and RLock per URN.
func StoreForIn[T any](r *Registry) *Store[T] {
	r.mu.RLock()
	v, ok := r.byType[reflect.TypeFor[T]()]
	r.mu.RUnlock()
	if !ok {
		return nil
	}
	s, _ := v.(*Store[T])
	return s
}

// ResolveUrnIn looks up a *T by URN in r.
func ResolveUrnIn[T any](r *Registry, u urn.URN) (*T, bool) {
	s := StoreForIn[T](r)
	if s == nil {
		return nil, false
	}
	return s.Get(u)
}

// ResolveUrnsIn looks up every URN in r, dropping the ones that don't resolve,
// so the result is NOT positionally parallel to urns. For a positional result
// (nil per unresolved URN), use Store.GetAllInto or EntityRefList.All.
// The store is looked up once, not per URN.
func ResolveUrnsIn[T any](r *Registry, urns []urn.URN) []*T {
	s := StoreForIn[T](r)
	if s == nil {
		return nil
	}
	return s.GetAll(urns)
}

// Build runs every registered store's hooks, in registration order (and,
// within a store, in the store's insertion order). Call once, after
// every Source's Load() has completed — hooks may freely resolve
// against any other store registered on r, since Build() guarantees
// every store is fully populated before any hook runs.
func (r *Registry) Build() error {
	timed := utils.Timed("[MODELS] Build (run store hooks)")
	defer timed()

	r.mu.RLock()
	snapshot := make([]runner, len(r.runners))
	copy(snapshot, r.runners)
	r.mu.RUnlock()

	for _, s := range snapshot {
		if err := s.runHooks(); err != nil {
			return err
		}
	}
	return nil
}

// ---- package-level default registry, for production call sites -----------

// Default is the registry EntityRef / EntityRefList resolve against.
var Default = NewRegistry()

// RegisterStore registers s against Default. See RegisterStoreIn.
func RegisterStore[T any](s *Store[T]) {
	RegisterStoreIn(Default, s)
}

// ResolveUrn looks up a *T by URN in Default.
func ResolveUrn[T any](u urn.URN) (*T, bool) {
	return ResolveUrnIn[T](Default, u)
}

// ResolveUrns looks up every URN in Default, dropping the ones that don't resolve.
func ResolveUrns[T any](urns []urn.URN) []*T {
	return ResolveUrnsIn[T](Default, urns)
}

// StoreFor returns the *Store[T] registered against Default, or nil.
func StoreFor[T any]() *Store[T] {
	return StoreForIn[T](Default)
}

func ResolveStore[T any]() *Store[T] {
	s := StoreForIn[T](Default)
	if s == nil {
		log.Printf("ResolveStore: no store registered for type %T", *new(T))
		return nil
	}
	return s
}

// Build runs every store registered against Default. Call once, after
// every Source's Load() has completed.
func Build() error {
	return Default.Build()
}

// Reset clears the Default registry so the dataset can be reloaded in place.
// See Registry.Reset.
func Reset() {
	Default.Reset()
}

// ResetDefaultForTest replaces Default with a fresh, empty registry.
// Test-only — production code should never call this; it exists so
// package tests (and consumers' own tests) can get a clean Default
// without state leaking between test functions.
func ResetDefaultForTest() {
	Default = NewRegistry()
}
