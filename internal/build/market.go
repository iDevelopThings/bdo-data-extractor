package build

import (
	"fmt"
	"sort"

	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

// marketKey groups items that share a central-market category. It uses more than
// (Category @5, equipType @7) because the market splits some items those two can't:
// within (Armor, Armor) sit real combat armor AND full-body life outfits
// (Functional/Crafted Clothes). The distinguishers are also real client fields — a
// combat piece fills ONE slot while an outfit fills the whole set (equipInfo.slots,
// byte @16-18), and Crafted Clothes are class-locked costumes while Functional
// Clothes are all-class (the class mask @77). So no DP/effects heuristics.
type marketKey struct {
	cat, typ    string
	multiSlot   bool // full-body outfit (fills the whole set) vs a single piece
	classLocked bool // class-restricted (costumes) vs all-class
}

func marketKeyOf(it *model.Item) (marketKey, bool) {
	if it.EquipInfo == nil {
		return marketKey{}, false
	}
	return marketKey{it.Category, it.EquipInfo.Type, len(it.EquipInfo.Slots) > 0, len(it.Classes) > 0}, true
}

// marketLearner derives marketCategory/marketSubCategory for equipment that never
// appears on the central market (Tuvala, boss gear, etc., whose records carry no
// market category) so it filters alongside listable items. observe learns a
// (main, sub) mapping from items with a real market category; fill applies it to
// the rest sharing the key where unambiguous (each target held by ≥90%).
type marketLearner struct {
	mains, subs map[marketKey]map[string]int
}

func newMarketLearner() *marketLearner {
	return &marketLearner{
		mains: map[marketKey]map[string]int{},
		subs:  map[marketKey]map[string]int{},
	}
}

func marketVote(m map[marketKey]map[string]int, k marketKey, v string) {
	if v == "" {
		return
	}
	if m[k] == nil {
		m[k] = map[string]int{}
	}
	m[k][v]++
}

// observe records an item's votes when it carries a real market category.
func (l *marketLearner) observe(it *model.Item) {
	if it.MarketCategory == "" {
		return
	}
	if k, ok := marketKeyOf(it); ok {
		marketVote(l.mains, k, it.MarketCategory)
		marketVote(l.subs, k, it.MarketSubCategory)
	}
}

func marketDominant(m map[string]int) (string, bool) {
	best, bn, tot := "", 0, 0
	for v, n := range m {
		tot += n
		if n > bn {
			best, bn = v, n
		}
	}
	return best, tot >= 3 && bn*100 >= tot*90
}

// fill sets the derived market category on an item that has none, returning true
// when it did.
func (l *marketLearner) fill(it *model.Item) bool {
	if it.MarketCategory != "" {
		return false
	}
	k, ok := marketKeyOf(it)
	if !ok {
		return false
	}
	main, ok := marketDominant(l.mains[k])
	if !ok {
		return false
	}
	it.MarketCategory = main
	if sub, ok := marketDominant(l.subs[k]); ok {
		it.MarketSubCategory = sub
	}
	return true
}

// buildMarketCategories registers the Central Market category tree (loc table 44) in
// the game's display order: main categories by id, sub-categories by sub id. These
// ids are the same @188/@189 values items carry in marketCategory/marketSubCategory.
func (b *Builder) buildMarketCategories() error {
	mainIDs := make([]int, 0, len(b.gs.MarketCats))
	for id := range b.gs.MarketCats {
		mainIDs = append(mainIDs, int(id))
	}
	sort.Ints(mainIDs)

	cats := make([]model.MarketCategory, 0, len(mainIDs))
	subCount := 0
	for _, id := range mainIDs {
		mc := b.gs.MarketCats[uint32(id)]
		subIDs := make([]int, 0, len(mc.Subs))
		for sid := range mc.Subs {
			subIDs = append(subIDs, int(sid))
		}
		sort.Ints(subIDs)
		subs := make([]model.MarketSubCategory, 0, len(subIDs))
		for _, sid := range subIDs {
			subs = append(subs, model.MarketSubCategory{ID: uint32(sid), Name: mc.Subs[uint32(sid)]})
		}
		subCount += len(subs)
		cats = append(cats, model.MarketCategory{ID: uint32(id), Name: mc.Name, SubCategories: subs})
	}

	p, err := b.addJSON("marketcategories.json", cats)
	if err != nil {
		return err
	}
	b.logf(fmt.Sprintf("market categories: %d mains, %d subs -> %s", len(cats), subCount, p))

	return nil
}
