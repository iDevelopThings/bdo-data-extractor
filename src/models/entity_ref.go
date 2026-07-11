package models

import (
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
	Values []*T      `json:"-"` // runtime resolution cache, not persisted
}

func NewEntityRefList[T any](urns ...urn.URN) EntityRefList[T] {
	return EntityRefList[T]{URNs: urns}
}

func (l *EntityRefList[T]) Len() int { return len(l.URNs) }

func (l *EntityRefList[T]) Add(u urn.URN) {
	l.URNs = append(l.URNs, u)
}

func (l *EntityRefList[T]) growValues() {
	if cap(l.Values) < len(l.URNs) {
		next := make([]*T, len(l.URNs))
		copy(next, l.Values)
		l.Values = next
		return
	}
	l.Values = l.Values[:len(l.URNs)]
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

// All resolves every entry (best-effort; entries that fail to resolve
// are simply nil in the result). Only call this when a consumer needs
// the fully expanded list — for the common "just need the URNs" path,
// read l.URNs directly instead.
func (l *EntityRefList[T]) All() []*T {
	out := make([]*T, len(l.URNs))
	for i := range l.URNs {
		out[i] = l.Get(i)
	}
	return out
}
