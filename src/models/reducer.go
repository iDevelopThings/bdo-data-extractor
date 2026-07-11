package models

// Reducer folds over every item in a Store during the SAME Build() pass
// that the store's other Hooks run in — no separate walk over items.
// Compare to View, which computes over store.All() on its own, on first
// Get(), independent of Build() ever running at all.
//
// Use Reducer when the aggregate needs to be ready the moment Build()
// completes and you want it computed alongside per-item hook work
// (icon assignment, ref linking, etc.) in one pass. Use View when the
// aggregate is a pure function of the raw loaded data and doesn't need
// to depend on Build() having run.
type Reducer[T, R any] struct {
	store *Store[T]
	acc   R
	step  func(acc R, item *T) R
}

// NewReducer registers step as a hook on store: step folds each item
// into acc, starting from initial, in the store's insertion order.
// Must be called before Build() runs for this store (same timing
// requirement as Store.AddHook, since it registers one internally).
func NewReducer[T, R any](store *Store[T], initial R, step func(acc R, item *T) R) *Reducer[T, R] {
	r := &Reducer[T, R]{store: store, acc: initial, step: step}
	store.AddHook(func(item *T) error {
		r.acc = r.step(r.acc, item)
		return nil
	})
	return r
}

// Result returns the accumulated value and whether it's actually ready
// (i.e. this store's Build() pass has completed). Before that, ok is
// false and the returned value is meaningless partial state — don't use
// it, check ok.
func (r *Reducer[T, R]) Result() (value R, ok bool) {
	return r.acc, r.store.Built()
}
