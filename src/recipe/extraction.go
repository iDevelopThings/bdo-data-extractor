// Package recipe holds the recipe-classification heuristics the build applies to
// the extracted recipe tables: telling backwards "extraction" recipes from real
// production ones (IsExtraction), flagging byproducts (MarkByproducts), and
// re-orienting imperial-delivery recipes (NormalizeImperialRecipes).
package recipe

import "github.com/idevelopthings/bdo-data-extractor/src/model"

// IsExtraction reports whether r is a backwards "extraction" recipe — a HEAT/GRIND
// that breaks a single finished item (count ≤ 1) down into a base material (e.g.
// Heating a piece of gear, or Grinding a reform stone, into Rough Sapphire) — as
// opposed to a real production recipe, which consumes several units of a material
// (Flax Thread ×10, Gold Ore ×5). Used to pick the real recipe over extractions and
// to decide which itemmaking candidates are genuinely gathered.
func IsExtraction(r model.Recipe) bool {
	return (r.Type == "HEAT" || r.Type == "GRIND") && len(r.Inputs) == 1 && r.Inputs[0].Count <= 1
}
