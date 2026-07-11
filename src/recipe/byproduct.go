package recipe

import (
	"fmt"
	"sort"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

// recipeSig is the language-independent identity of a recipe: its type plus its
// sorted (item*count) input multiset. Two recipes with the same signature are the
// same craft — the key to spotting a recipe that was copied onto a byproduct's
// item page (the per-item recipe XMLs list the producing recipe on every item it
// can yield, main product or byproduct, with no marker distinguishing them).
func recipeSig(r model.Recipe) string {
	parts := make([]string, len(r.Inputs))
	for i, in := range r.Inputs {
		parts[i] = fmt.Sprintf("%d*%d", in.Item.ID(), in.Count)
	}
	sort.Strings(parts)
	return r.Type + "|" + strings.Join(parts, ",")
}

// MarkByproducts sets Recipe.ByproductOf on every recipe that does not really
// craft its own Output — the item just procs from it as a low-chance byproduct.
//
// Detected purely structurally (no description/name text, so it is
// localization-independent): a recipe R listed on item Y is a byproduct when R is
// identical to the recipe of an ingredient X (X != Y) that Y itself consumes in
// one of its recipes. That means crafting X is what the recipe actually does, and
// Y falls out of it as a byproduct. Example: "Standardized Timber Square" lists a
// [Log ×10] recipe, but [Log ×10] is exactly "Usable Scantling"'s recipe and
// Usable Scantling is an ingredient of the Timber Square's real recipe — so
// chopping Logs really makes Usable Scantling and the Square just procs off it.
//
// The rule fires only on this "byproduct of one of my own ingredients" tier
// chain (Timber/Grain Juice/Whale Tendon families), which is the case that
// pollutes the craft tree; it deliberately does NOT touch parallel variants
// (Grilled vs Smoked Sausage, X vs Ultimate X) whose main-vs-byproduct identity
// is not present anywhere in the client. Mutates recipes in place, returns the
// number flagged.
func MarkByproducts(recipes []model.Recipe) int {
	// per output item: the signatures it produces, and every ingredient its
	// recipes consume.
	sigs := map[uint32]map[string]bool{}
	ingredients := map[uint32]map[uint32]bool{}
	for i := range recipes {
		out := recipes[i].Output.ID()
		if sigs[out] == nil {
			sigs[out] = map[string]bool{}
			ingredients[out] = map[uint32]bool{}
		}
		sigs[out][recipeSig(recipes[i])] = true
		for _, in := range recipes[i].Inputs {
			ingredients[out][in.Item.ID()] = true
		}
	}

	marked := 0
	for i := range recipes {
		out := recipes[i].Output.ID()
		rsig := recipeSig(recipes[i])
		self := make(map[uint32]bool, len(recipes[i].Inputs))
		for _, in := range recipes[i].Inputs {
			self[in.Item.ID()] = true
		}
		for x := range ingredients[out] {
			if x == out || self[x] { // an ingredient can't be the item itself or feed its own recipe
				continue
			}
			if sigs[x][rsig] { // this recipe is really X's recipe → Y is X's byproduct
				recipes[i].ByproductOf = model.ItemRef(x)
				marked++
				break
			}
		}
	}
	return marked
}
