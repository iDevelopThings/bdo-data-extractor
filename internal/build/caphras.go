package build

import (
	"fmt"
	"math"
	"slices"

	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

// buildCaphras writes the Caphras Enhancement chart (cronenchant.bss — the
// game's internal name for the Caphras system): per category (cronKey 1..10)
// and enhancement level (18/19/20), the 20 Caphras levels' stone costs and
// added stats. The item→category mapping is computed inside the game client,
// not stored in the data files, so it is not part of this output.
func (b *Builder) buildCaphras() error {
	cats := b.caphras // decoded during buildItems (deriveCaphrasCategories)
	if cats == nil {
		data, err := b.src.Read("cronenchant.bss")
		if err != nil {
			return err
		}
		cats, err = tables.DecodeCaphras(data)
		if err != nil {
			return err
		}
	}
	p, err := b.write("caphras.json", cats)
	if err != nil {
		return err
	}
	b.logf(fmt.Sprintf("caphras: %d categories × %d enhancement levels × 20 steps -> %s", len(cats), len(cats[0].Levels), p))

	return nil
}

// caphrasChart maps caphras category -> enchant level -> the steps at that level,
// so each level's steps can be hung directly on the item's matching EnchantLevel.
type caphrasChart map[int]map[int][]model.CaphrasLevel

// loadCaphrasChart reads cronenchant.bss, caches the decoded categories on the
// Builder, and returns the category/enhancement-level index.
func (b *Builder) loadCaphrasChart() (caphrasChart, error) {
	data, err := b.src.Read("cronenchant.bss")
	if err != nil {
		return nil, fmt.Errorf("read cronenchant: %w", err)
	}
	cats, err := tables.DecodeCaphras(data)
	if err != nil {
		return nil, err
	}
	b.caphras = cats
	chart := make(caphrasChart, len(cats))
	for _, c := range cats {
		byLevel := make(map[int][]model.CaphrasLevel, len(c.Levels))
		for _, lv := range c.Levels {
			byLevel[lv.EnchantLevel] = lv.Steps
		}
		chart[c.Key] = byLevel
	}
	return chart, nil
}

// applyCaphras stamps the Caphras category and its per-level chart onto an eligible
// item's enhancement, returning true when it did. The game computes the category
// inside the executable — it is stored nowhere in the data files (see FORMATS §16) —
// but it follows the equipment taxonomy exactly: slot kind × price tier, the buy
// price separating tiers in disjoint bands (boss 10M+, blue 2M+, green below).
// Eligibility = max-enhancement-20 weapon/armor of green grade or better, excluding
// multi-slot life outfits.
//
// Validated chart-exact against 1,315 community-labeled items, with no eligible item
// missed and no ineligible one matched. The only label disagreements are boss
// awakening weapons the community labels "mainhand" — and categories 1 and 2 (the
// awakening vs non-awakening boss main-weapon split below) share an identical chart
// anyway.
func applyCaphras(it *model.Item, chart caphrasChart, maxlv map[uint32]int) bool {
	if chart == nil || it.Enhancement == nil || it.EquipInfo == nil || maxlv[it.ID] != 20 {
		return false
	}
	if it.Grade == model.ItemGradeWhite || len(it.EquipInfo.Slots) > 0 {
		return false
	}
	tier := 0 // green
	switch {
	case it.BuyPrice >= 10_000_000:
		tier = 2 // boss
	case it.BuyPrice >= 2_000_000:
		tier = 1 // blue
	}
	cat := 0
	switch it.Category {
	case "MainWeapon":
		switch {
		case tier == 2 && it.EquipInfo.Slot == model.SlotNameAwakeningWeapon:
			cat = 2
		case tier == 2:
			cat = 1
		case tier == 1:
			cat = 3
		default:
			cat = 4
		}
	case "SubWeapon":
		if tier == 2 {
			cat = 5
		} else {
			cat = 6
		}
	case "Armor":
		switch tier {
		case 2:
			cat = 7
		case 1:
			cat = 9
		default:
			cat = 10
		}
	}
	if cat == 0 {
		return false
	}
	// The enchant curve is shared across every item of a baseId, but the Caphras
	// category is per-item (a mainhand and its offhand can share a curve yet map to
	// different categories), so give this item its own copy before stamping.
	enh := *it.Enhancement
	enh.Levels = slices.Clone(it.Enhancement.Levels)
	enh.CaphrasCategory = model.CaphrasRef(cat)
	byLevel := chart[cat]
	for i := range enh.Levels {
		minLvl, maxLvl := math.MaxInt, math.MinInt
		if steps := byLevel[enh.Levels[i].Level]; steps != nil {
			enh.Levels[i].Caphras = steps
			for _, s := range steps {
				if s.Level < minLvl {
					minLvl = s.Level
				}
				if s.Level > maxLvl {
					maxLvl = s.Level
				}
			}
			enh.Levels[i].CaphrasMinLevel = minLvl
			enh.Levels[i].CaphrasMaxLevel = maxLvl
		}
	}
	it.Enhancement = &enh
	return true
}
