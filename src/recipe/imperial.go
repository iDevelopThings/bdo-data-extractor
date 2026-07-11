package recipe

import (
	"fmt"
	"sort"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

// NormalizeImperialRecipes fixes the direction of "imperial delivery" recipes.
//
// Imperial crafting (the ROYALGIFT_ALCHEMY / ROYALGIFT_COOK types) packages a
// finished good INTO a delivery box — a SpecialGoods item — to sell to the Imperial
// Trader, so the box is always the recipe's OUTPUT. But the per-item XMLs carry the
// `<manufacture action="MANUFACTURE_ROYALGIFT_*">` block on BOTH pages, each listing
// the other item:
//
//	672.xml  (Elixir of Frenzy):  <manufacture ROYALGIFT><item>9836 (box)</item>
//	9836.xml (the box):           <manufacture ROYALGIFT><item>672 (elixir)</item>
//
// Our parser assumes the page item is the output, so the elixir's page yields a
// bogus "Elixir of Frenzy ← its own delivery box". Rather than drop that, we
// re-orient any ROYALGIFT recipe whose output isn't the box so the SpecialGoods box
// is the output and the good is the input — the correct "box ← good" direction. The
// two pages then produce the same recipe, so identical imperial recipes are deduped.
// This keeps the good's "used in → box" link even when only the good's page carried
// the relationship. Returns the recipes plus how many were re-oriented.
func NormalizeImperialRecipes(recipes []model.Recipe, isSpecialGoods func(uint32) bool) ([]model.Recipe, int) {
	out := recipes[:0]
	seen := map[string]bool{}
	oriented := 0
	for _, r := range recipes {
		if strings.HasPrefix(r.Type, "ROYALGIFT_") {
			if !isSpecialGoods(r.Output.ID()) {
				if box := specialGoodsInput(r, isSpecialGoods); box != 0 {
					// the box is the real output; the page item is the ingredient.
					// counts here are unreliable (the game data omits the per-box
					// input count), so leave it 0 = unknown, matching the box page.
					r = model.Recipe{Type: r.Type, Output: model.ItemRef(box), Inputs: []model.Ingredient{{Item: r.Output}}}
					oriented++
				}
			}
			key := imperialKey(r)
			if seen[key] {
				continue // both pages describe the same delivery; keep one
			}
			seen[key] = true
		}
		out = append(out, r)
	}
	return out, oriented
}

func specialGoodsInput(r model.Recipe, isSpecialGoods func(uint32) bool) uint32 {
	for _, in := range r.Inputs {
		if isSpecialGoods(in.Item.ID()) {
			return in.Item.ID()
		}
	}
	return 0
}

// imperialKey identifies a ROYALGIFT recipe for dedup: type + output + its input
// item set (counts ignored — imperial input counts aren't in the data).
func imperialKey(r model.Recipe) string {
	ins := make([]string, len(r.Inputs))
	for i, in := range r.Inputs {
		ins[i] = fmt.Sprint(in.Item.ID())
	}
	sort.Strings(ins)
	return fmt.Sprintf("%s|%d|%s", r.Type, r.Output.ID(), strings.Join(ins, ","))
}
