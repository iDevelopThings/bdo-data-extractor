package models

import (
	"slices"

	"github.com/idevelopthings/bdo-data-extractor/src/urn"
)

// EntityRef is a lazily-resolved foreign key: URN is the durable,
// serialized identity; Value is a runtime-only cache populated on first
// GetValue() call via whichever Store[T] was registered for T.
type EntityRef[T any] struct {
	urn.URN `json:"urn"`
	Value   *T `json:"value,omitempty"`
}

func NewEntityRef[T any](u urn.URN) *EntityRef[T] {
	return &EntityRef[T]{URN: u}
}

// MarshalText encodes a ref as its bare URN string. Defined explicitly (as a
// TextMarshaler, not json.Marshaler) so the ref serializes compactly AND Wails'
// TS generator types EntityRef[T] as `string` instead of `any`. The runtime
// Value cache is intentionally not serialized.
func (e EntityRef[T]) MarshalText() ([]byte, error) {
	return e.URN.MarshalText()
}

func (e *EntityRef[T]) UnmarshalText(text []byte) error {
	return e.URN.UnmarshalText(text)
}

// GetValue resolves and memoizes the referenced value on first access.
// Repeated calls after the first are a plain nil-check, no store lookup.
func (e *EntityRef[T]) GetValue() *T {
	if e == nil {
		return nil
	}
	if e.Value == nil && e.URN.IsValid() {
		e.Value, _ = ResolveUrn[T](e.URN)
	}
	return e.Value
}

// SetValue sets both the cached value and the URN it corresponds to.
func (e *EntityRef[T]) SetValue(v *T, u urn.URN) {
	e.Value = v
	e.URN = u
}

// SetURN repoints the ref at a new URN and invalidates any cached value,
// forcing a re-resolve on the next GetValue() call.
func (e *EntityRef[T]) SetURN(u urn.URN) {
	e.URN = u
	e.Value = nil
}

// ID returns the ref's numeric id (its URN's trailing id part), or 0 for a nil
// or invalid ref. It's the bridge for id-keyed code that hasn't moved to URNs —
// callers that know the ref is present can use it directly.
func (e *EntityRef[T]) ID() uint32 {
	if e == nil || !e.URN.IsValid() {
		return 0
	}
	n, _ := e.URN.Uint32()
	return n
}

// EntityRefList is the struct-of-arrays counterpart to []*EntityRef[T]:
// one URN slice, one lazily/partially-populated Value slice, instead of
// N separate EntityRef allocations. Resolution is still per-element and
// on-demand — Get(i) only resolves index i; All() resolves everything,
// so only call it when a consumer actually wants the full expansion.
type EntityRefList[T any] struct {
	URNs   []urn.URN `json:"urns"`
	Values []*T      `json:"-"` // runtime resolution cache, parallel to URNs once sized

	// resolved is set once every URN has been looked up. Values being full-length
	// doesn't imply that — a single Get(i) sizes it — so the full-resolve paths
	// need their own flag to avoid returning a half-populated cache.
	resolved bool
}

func NewEntityRefList[T any](urns ...urn.URN) EntityRefList[T] {
	return EntityRefList[T]{URNs: urns}
}

func (l *EntityRefList[T]) Len() int { return len(l.URNs) }

func (l *EntityRefList[T]) Add(u urn.URN) {
	l.URNs = append(l.URNs, u)
	l.resolved = false
}

// IndexOf returns the position of u, or -1 if absent.
func (l *EntityRefList[T]) IndexOf(u urn.URN) int {
	return slices.Index(l.URNs, u)
}

// Contains reports whether u is already in the list.
func (l *EntityRefList[T]) Contains(u urn.URN) bool {
	return slices.Contains(l.URNs, u)
}

// AddUnique appends u only if it isn't already present, reporting whether it was
// added.
func (l *EntityRefList[T]) AddUnique(u urn.URN) bool {
	if l.Contains(u) {
		return false
	}
	l.Add(u)
	return true
}

// Remove drops the first entry equal to u, keeping the resolution cache
// index-parallel by deleting the same slot (the cached *T for the surviving
// entries — and any already handed out — stay valid; they point at Store-owned
// values, not into these slices). Reports whether an entry was removed.
func (l *EntityRefList[T]) Remove(u urn.URN) bool {
	i := slices.Index(l.URNs, u)
	if i < 0 {
		return false
	}
	l.URNs = slices.Delete(l.URNs, i, i+1)
	if i < len(l.Values) {
		l.Values = slices.Delete(l.Values, i, i+1)
	}
	return true
}

// growValues sizes the Values cache to match URNs, preserving what's cached.
func (l *EntityRefList[T]) growValues() {
	if len(l.Values) == len(l.URNs) {
		return
	}
	if cap(l.Values) >= len(l.URNs) {
		l.Values = l.Values[:len(l.URNs)]
		return
	}
	next := make([]*T, len(l.URNs))
	copy(next, l.Values)
	l.Values = next
}

// Get resolves and memoizes index i. Returns nil for an out-of-range
// index or a URN that fails to resolve.
func (l *EntityRefList[T]) Get(i int) *T {
	if i < 0 || i >= len(l.URNs) {
		return nil
	}
	if i < len(l.Values) && l.Values[i] != nil {
		return l.Values[i]
	}
	v, ok := ResolveUrn[T](l.URNs[i])
	if !ok {
		return nil
	}
	l.growValues()
	l.Values[i] = v
	return v
}

// GetManyByIndex resolves and memoizes every index in indices, returning a slice
// parallel to indices: out[n] is the entity for indices[n], or nil if that index
// is out of range or its URN doesn't resolve. The store is looked up at most once
// for the whole batch, and only for indices that aren't already cached.
func (l *EntityRefList[T]) GetManyByIndex(indices []int) []*T {
	if l == nil || len(indices) == 0 {
		return nil
	}

	out := make([]*T, len(indices))

	// Resolved lazily on the first cache miss, then reused: a fully-cached batch
	// pays no registry lookup at all, and a partial one pays exactly one.
	var store *Store[T]

	for n, i := range indices {
		if i < 0 || i >= len(l.URNs) {
			continue
		}
		if i < len(l.Values) && l.Values[i] != nil {
			out[n] = l.Values[i]
			continue
		}
		if store == nil {
			if store = StoreFor[T](); store == nil {
				return out
			}
			l.growValues()
		}
		v := store.GetUnsafe(l.URNs[i])
		if v == nil {
			continue
		}
		l.Values[i] = v
		out[n] = v
	}

	return out
}

// resolveAll looks the store up once and fills the Values cache positionally,
// skipping entries already resolved. Entries whose URN doesn't resolve stay nil.
func (l *EntityRefList[T]) resolveAll() []*T {
	if l == nil || len(l.URNs) == 0 {
		return nil
	}
	if l.resolved {
		return l.Values
	}

	l.growValues()

	store := StoreFor[T]()
	if store == nil {
		return l.Values
	}

	for i, u := range l.URNs {
		if l.Values[i] == nil {
			l.Values[i] = store.GetUnsafe(u)
		}
	}
	l.resolved = true

	return l.Values
}

// All resolves every entry (best-effort; entries that fail to resolve are simply
// nil in the result, which stays parallel to URNs). Only call this when a consumer
// needs the fully expanded list — for the common "just need the URNs" path, read
// l.URNs directly instead. The returned slice is the list's own cache: read-only.
func (l *EntityRefList[T]) All() []*T {
	return l.resolveAll()
}

// AllBulk resolves the whole list in one pass. Prefer it over Get(i) in a loop:
// it pays the registry lookup once rather than per element.
func (l *EntityRefList[T]) AllBulk() []*T {
	return l.resolveAll()
}
