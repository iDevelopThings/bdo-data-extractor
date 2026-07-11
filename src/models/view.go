package models

import "sync"

// View is a lazily computed, memoized derived dataset over a Store's
// items — sorted lists, grouped counts, secondary-key lookup indexes,
// and so on. build runs at most once, on the first Get() call, no
// matter how many callers ask for it afterward.
type View[T, R any] struct {
	store *Store[T]
	build func([]*T) R
	once  sync.Once
	value R
}

// NewView creates a View backed by store, computed by build on first
// Get(). build receives store.All() (insertion order) — since that
// slice may be shared by other Views over the same store, build should
// clone before sorting or otherwise mutating it in place.
func NewView[T, R any](store *Store[T], build func([]*T) R) *View[T, R] {
	return &View[T, R]{store: store, build: build}
}

// Get returns the memoized derived value, computing it on first call.
func (v *View[T, R]) Get() R {
	v.once.Do(
		func() {
			v.value = v.build(v.store.All())
		},
	)
	return v.value
}
