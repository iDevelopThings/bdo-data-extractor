package tables

import "math"

// rnd rounds a float to the nearest integer.
func rnd(v float64) int { return int(math.Round(v)) }

// round2 rounds a coordinate to 2 decimals for compact JSON.
func round2(f float64) float64 { return math.Round(f*100) / 100 }

// dev returns a pointer to v when it differs from the field's dominant default,
// else nil — every unidentified item-row byte is read into a typed deviation-only
// field (model.ItemUnknowns), so a typical item carries none and the JSON stays
// sparse while the value is still visible on the items that are unusual for it.
func dev(v, def int) *int {
	if v == def {
		return nil
	}
	return &v
}
