package build

import (
	"fmt"
	"os"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/config"
	"github.com/idevelopthings/bdo-data-extractor/internal/jsonio"
	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
	"github.com/idevelopthings/bdo-data-extractor/src/models"
	"github.com/idevelopthings/bdo-data-extractor/src/utils"
)

// buildItems decodes the item-stat, max-level, buff/skill, and enchant tables,
// merges them into one Item per id, flags gathered materials, and writes items.json.
func (b *Builder) buildItems() error {
	encOff, err := b.src.Read("itemenchantoffset.dbss")
	if err != nil {
		return err
	}
	encDat, err := b.src.Read("itemenchant.dbss")
	if err != nil {
		return err
	}
	stats, err := tables.DecodeItemStats(encOff, encDat)
	if err != nil {
		return err
	}
	b.logf(fmt.Sprintf("itemenchant: %d item stat rows", len(stats)))

	mlOff, err := b.src.Read("itemmaxleveloffset.dbss")
	if err != nil {
		return err
	}
	mlDat, err := b.src.Read("itemmaxlevel.dbss")
	if err != nil {
		return err
	}
	maxlv, err := tables.DecodeMaxLevels(mlOff, mlDat)
	if err != nil {
		return err
	}
	b.logf(fmt.Sprintf("itemmaxlevel: %d rows", len(maxlv)))

	// consumable effect chain: item->skill->buff (typed effects + cooldown)
	buffDat, err := b.src.Read("buff.dbss")
	if err != nil {
		return err
	}
	buffs, err := tables.DecodeBuffs(buffDat)
	if err != nil {
		return err
	}
	skillOff, err := b.src.Read("skilloffset.dbss")
	if err != nil {
		return err
	}
	skillDat, err := b.src.Read("skill.dbss")
	if err != nil {
		return err
	}
	skills, err := tables.DecodeSkillEffects(skillOff, skillDat, buffs)
	if err != nil {
		return err
	}
	b.logf(fmt.Sprintf("buffs: %d, skills: %d", len(buffs), len(skills)))

	essOff, err := b.src.Read("enchantstaticstatusoffset.dbss")
	if err != nil {
		return err
	}
	essDat, err := b.src.Read("enchantstaticstatus.dbss")
	if err != nil {
		return err
	}
	curves, err := tables.DecodeEnchantCurves(essOff, essDat)
	if err != nil {
		return err
	}
	links, err := tables.EnchantLinks(encOff, encDat, curves, maxlv)
	if err != nil {
		return err
	}
	b.logf(fmt.Sprintf("enchant curves: %d, item->enchant links: %d", len(curves), len(links)))

	b.items = b.mergeItems(stats, maxlv, buffs, skills, curves, links)
	b.logf(fmt.Sprintf("icons: backfilled %d name-only items from id-named archive icons", b.backfillShellIcons()))
	b.scanItemInfo() // populates b.recipes (needed to tell raw mats from processed)

	gathered := b.flagGathered()
	b.logf(fmt.Sprintf("itemmaking: flagged %d gathered items", gathered))

	untradable, caphrasItems, nv, ng := b.finalizeItems(maxlv)
	b.logf(fmt.Sprintf("market categories: derived for %d untradeable equipment items", untradable))
	b.logf(fmt.Sprintf("caphras: derived chart categories for %d items", caphrasItems))
	b.logf(fmt.Sprintf("item variants: %d copies linked to canonical, %d ghost records flagged", nv, ng))

	// Write items.json + item_enhancements.json (the 400MB+ bulk of the output) in
	// the background while the sidecar stages build; Run joins via awaitWrite. Safe
	// because b.items/b.enhancements are frozen after this point — later stages only
	// read them.
	b.writeDone = make(chan writeResult, 1)
	go func() {
		n, size, err := b.writeItems()
		b.writeDone <- writeResult{n, size, err}
	}()

	return nil
}

// writeResult carries the outcome of the background items/enhancements write.
type writeResult struct {
	n    int
	size int64
	err  error
}

// awaitWrite blocks until the background writeItems started by buildItems finishes,
// logs the result, and returns its error. It is a no-op (and safe to call again) if
// no write is pending.
func (b *Builder) awaitWrite() error {
	if b.writeDone == nil {
		return nil
	}
	res := <-b.writeDone
	b.writeDone = nil
	if res.err != nil {
		return res.err
	}
	b.logf(fmt.Sprintf("wrote %d items -> %s (%.1f MB)", res.n, "items.json", float64(res.size)/1e6))

	return nil
}

// mergeItems folds every decoded source into one Item per id.
func (b *Builder) mergeItems(
	stats map[uint32]tables.ItemStat,
	maxlv map[uint32]int,
	buffs map[uint16]tables.Buff,
	skills map[uint32]tables.SkillEffect,
	curves map[uint32]*model.Enhancement,
	links map[uint32]uint32,
) map[uint32]*model.Item {

	items := make(map[uint32]*model.Item, len(stats))
	get := func(id uint32) *model.Item {
		it := items[id]
		if it == nil {
			it = &model.Item{
				BaseFor: models.NewBaseFor[model.Item](id),
				ID:      id,
			}
			items[id] = it
		}
		return it
	}

	for id, s := range stats {
		it := get(id)
		it.Weight = s.Weight
		it.BuyPrice = s.Buy
		it.SellPrice = s.Sell
		it.RepairPrice = s.Repair
		it.MaxDurability = s.MaxDurability
		it.Classes = s.Classes
		it.Grade = s.Grade
		it.Category = s.Category
		it.ItemType = s.ItemType
		it.Icon = s.Icon
		it.ExpirationMinutes = s.Expiration
		it.RequiredLevel = s.RequiredLevel
		it.MaxStack = s.MaxStack
		it.DyeParts = s.DyeParts
		it.Marketable = s.Marketable
		it.BindType = s.BindType
		it.MarketRegisterLimit = s.MarketRegisterLimit
		it.ItemMaterial = s.ItemMaterial
		it.Stackable = s.Stackable
		it.ApplyDirectly = s.ApplyDirectly
		it.VestedType = s.VestedType
		it.UserVested = s.UserVested
		it.ForTrade = s.ForTrade
		it.TradeType = s.TradeType
		it.LifeExpType = s.LifeExpType
		it.EventType = s.EventType
		it.EventParam1 = s.EventParam1
		it.EventParam2 = s.EventParam2
		it.ShownInNote = s.ShownInNote
		it.Cash = s.Cash
		it.CronEnchant = s.CronEnchant
		it.Dyeable = s.Dyeable
		it.PersonalTrade = s.PersonalTrade
		it.NodeFreeTrade = s.NodeFreeTrade
		it.FamilyInventory = s.FamilyInventory
		it.ContributionCost = s.ContributionCost
		it.ItemUnknowns = s.U
		if s.JewelGroup >= 0 {
			if g, ok := b.gs.JewelGroups[uint32(s.JewelGroup)]; ok {
				it.CrystalGroup = &model.CrystalGroup{Name: g.Name, Max: g.Max}
			}
		}
		if s.ItemType == "Equip" { // group the equip slot taxonomy (@14 slot / @15 kind / @7 type)
			typ := s.EquipType
			if typ == "None" {
				typ = ""
			}
			ei := &model.EquipInfo{Slot: s.Slot, Kind: s.Kind, Type: typ}
			if len(s.ExtraSlots) > 0 { // multi-slot costume: full occupied-slot list
				ei.Slots = append([]string{s.Slot}, s.ExtraSlots...)
			}
			it.EquipInfo = ei
		}
		if mc, ok := b.gs.MarketCats[uint32(s.MarketCatID)]; ok {
			it.MarketCategory = mc.Name
			it.MarketSubCategory = mc.Subs[uint32(s.MarketSubID)]
		}
		if se, ok := skills[s.SkillKey]; ok && s.SkillKey != 0 {
			it.Effects = b.buildEffects(buffs, se)
		}
	}
	for id, lv := range maxlv {
		get(id).MaxEnhance = new(lv)
	}
	for id, nm := range b.gs.Names {
		get(id).Name = nm
	}
	for id, d := range b.gs.Descs {
		get(id).Description = d
	}
	for id, t := range b.gs.UseTexts {
		get(id).UseText = t
	}
	for id, t := range b.gs.ExchangeTexts {
		get(id).ExchangeInfo = t
	}
	for id, base := range links {
		if c := curves[base]; c != nil {
			it := get(id)

			labelEnchantLevels(c, it.Category)
			it.Enhancement = c
		}
	}
	return items
}

// backfillShellIcons fills Icon for items the stat table gave no icon — mostly
// name-only shells that have a localized name but no itemenchant.dbss record — from
// the archive's id-named icons at ui_texture/icon/new_icon/<...>/<zero-padded id>.dds.
// The path stored is relative to ui_texture/icon/, matching the stat-record icons the
// icon extractor already resolves. Items with no such archive icon stay iconless.
// Returns the number backfilled.
func (b *Builder) backfillShellIcons() int {
	const prefix = "ui_texture/icon/"
	byID := map[uint32]string{}
	for i := range b.src.Index.Files {
		p := strings.ToLower(b.src.Index.Path(i))
		if !strings.HasPrefix(p, prefix+"new_icon/") || !strings.HasSuffix(p, ".dds") {
			continue
		}
		n, err := strconv.ParseUint(strings.TrimSuffix(path.Base(p), ".dds"), 10, 32)
		if err != nil {
			continue
		}
		if _, ok := byID[uint32(n)]; !ok {
			byID[uint32(n)] = p[len(prefix):]
		}
	}
	filled := 0
	for id, it := range b.items {
		if it.Icon == "" {
			if rel, ok := byID[id]; ok {
				it.Icon = rel
				filled++
			}
		}
	}
	return filled
}

// finalizeItems runs the post-merge item pipeline in two passes over b.items
// (down from five): a learn/group pass collecting market-category votes and variant
// groups, then an apply pass that fills derived market categories, stamps caphras
// charts, and moves enchant curves to the sidecar. Per-item writes are independent,
// so folding them into one pass is order-safe; the counts are returned for logging.
func (b *Builder) finalizeItems(maxlv map[uint32]int) (untradable, caphras, variants, ghosts int) {
	market := newMarketLearner()
	groups := map[vkey][]*model.Item{}
	for _, it := range b.items {
		market.observe(it)
		if it.Name == "" {
			continue
		}
		if it.ItemType == "" && it.Icon == "" { // a loc name with no item data: a ghost record
			it.Ghost = true
			ghosts++
			continue
		}
		slot := ""
		if it.EquipInfo != nil {
			slot = it.EquipInfo.Slot
		}
		k := vkey{it.Name, it.ItemType, it.Category, it.Grade, it.Icon, slot}
		groups[k] = append(groups[k], it)
	}
	variants = assignVariants(groups)

	chart := b.loadCaphrasChart()

	b.enhancements = make(map[uint32]*model.Enhancement)
	for _, it := range b.items {
		if market.fill(it) {
			untradable++
		}
		if applyCaphras(it, chart, maxlv) {
			caphras++
		}
		b.stashEnhancement(it)
	}
	return untradable, caphras, variants, ghosts
}

// stashEnhancement moves an item's full enchant curve into b.enhancements (written
// to item_enhancements.json) and leaves the item a level-stripped copy so items.json
// stays slim.
func (b *Builder) stashEnhancement(it *model.Item) {
	if it.Enhancement == nil {
		return
	}
	cloned := new(model.Enhancement)
	*cloned = *it.Enhancement
	cloned.BaseFor = models.NewBaseFor[model.Enhancement](it.ID)

	e := *cloned
	e.Levels = []model.EnchantLevel{}
	it.Enhancement = &e

	b.enhancements[it.ID] = cloned
}

// labelEnchantLevels sets each level's display Name from the enhancement scheme. The
// scheme is roman-from-1 (level 1 = PRI, no numeric "+1..+15" phase) when EITHER:
//   - the item is an accessory — accessories always enhance PRI→PEN/DEC directly, even
//     the guaranteed Tuvala/season lines (chance 1.0 but still roman); OR
//   - reaching level 1 isn't guaranteed — BDO always grants "+1" (100%) but never
//     "PRI", so a non-accessory whose level-1 chance is <1.0 is a roman line like
//     Sovereign / Fallen God.
//
// Otherwise it's the numeric phase (roman tiers start at 16). Neither signal works
// alone: maxEnhance/category can't tell a +1..+5 basic piece from a PRI..PEN accessory
// (both cap at 5), and the chance alone mislabels guaranteed accessories — together
// they're exact. The curve is shared across items of a baseId, so re-labeling is
// idempotent.
//
// (Some boss lines — Fallen God, Labreska — additionally show *named* stages like
// "Obliterating" instead of the roman tier; that's a separate display layer sourced
// from the enhanced item's name, not encoded here.)
func labelEnchantLevels(enh *model.Enhancement, category string) {
	romanFromOne := category == "Accessory"
	if !romanFromOne {
		for _, lv := range enh.Levels {
			if lv.Level == 1 {
				romanFromOne = lv.EnhanceChance < 1.0
				break
			}
		}
	}
	romanStart := 16
	if romanFromOne {
		romanStart = 1
	}
	for i := range enh.Levels {
		enh.Levels[i].Name = model.EnhanceLevelName(enh.Levels[i].Level, romanStart)
	}
}

// buildEffects turns a consumable's buff list into stat lines. Per buff, in
// order of reliability: (1) the STRUCTURED effect decoded from the buff's
// module parameter block (tables.ResolveBuffStat — pure binary, covers the
// Korean-only component buffs like draught effects); (2) the buff's English
// loc name when it parses as a single stat, for modules not in the static
// table; (3) the Korean-name parser, kept as "hidden" effects. Duration is
// the longest buff duration among the resolved effects. Returns nil if empty.
func (b *Builder) buildEffects(buffs map[uint16]tables.Buff, se tables.SkillEffect) *model.Effects {
	var stats, hidden []model.StatMod
	dur := 0
	bump := func(bid uint16) {
		if d := buffs[bid].DurationMs; d > dur {
			dur = d
		}
	}
	for _, bid := range se.Buffs {
		if stat, op, val, unit, ok := tables.ResolveBuffStat(buffs[bid]); ok {
			stats = append(stats, model.StatMod{Stat: stat, Op: op, Value: val, Unit: unit, Buff: uint32(bid)})
			bump(bid)
			continue
		}
		if name := b.gs.BuffNames[uint32(bid)]; name != "" {
			if stat, op, val, unit, ok := tables.ParseStat(name); ok {
				stats = append(stats, model.StatMod{Stat: stat, Op: op, Value: val, Unit: unit, Buff: uint32(bid)})
				bump(bid)
			}
			continue // named but non-stat (e.g. the Satiated debuff): drop
		}
		if stat, op, val, unit, ok := tables.ParseHiddenStat(buffs[bid].NameKR); ok {
			hidden = append(hidden, model.StatMod{Stat: stat, Op: op, Value: val, Unit: unit, Buff: uint32(bid)})
			bump(bid)
		}
	}
	if len(stats) == 0 && len(hidden) == 0 {
		return nil
	}

	statGroup, hiddenGroup := model.EffectGroup{}, model.EffectGroup{}
	if len(stats) > 0 {
		statGroup.Title = "Stats"
		statGroup.Stats = stats
	}
	if len(hidden) > 0 {
		hiddenGroup.Title = "Hidden"
		hiddenGroup.Stats = hidden
	}

	return &model.Effects{
		CooldownMs: se.CooldownMs,
		DurationMs: dur,
		Stats:      statGroup,
		Hidden:     hiddenGroup,
	}
}

// writeItems sorts items by id and writes them to outPath. Returns the item
// count and the written file size.
func (b *Builder) writeItems() (count int, size int64, err error) {
	timed := utils.Timed(fmt.Sprintf("writeItems: %d items + %d enhancements", len(b.items), len(b.enhancements)))
	defer timed()

	_, itemsPath := b.outFilePath("items.json")
	_, enhancementsPath := b.outFilePath("item_enhancements.json")

	ids := make([]uint32, 0, len(b.items))
	eIds := make([]uint32, 0, len(b.enhancements))

	for id := range b.items {
		ids = append(ids, id)
	}
	for id := range b.enhancements {
		eIds = append(eIds, id)
	}

	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	sort.Slice(eIds, func(i, j int) bool { return eIds[i] < eIds[j] })

	list := make([]*model.Item, len(ids))
	enhancements := make([]*model.Enhancement, len(eIds))

	type DumpData struct {
		Item        *model.Item        `json:"items"`
		Enhancement *model.Enhancement `json:"enhancements"`
	}
	toDump := make(map[uint32]DumpData)

	for i, id := range ids {
		list[i] = b.items[id]

		if config.GlobalConfig.DumpItemIds != nil {
			if slices.Contains(config.GlobalConfig.DumpItemIds, id) {
				toDump[id] = DumpData{
					Item:        b.items[id],
					Enhancement: b.enhancements[id],
				}
			}
		}
	}

	for i, id := range eIds {
		enhancements[i] = b.enhancements[id]
	}

	if err := jsonio.WriteArray(itemsPath, list, *config.GlobalConfig.Pretty); err != nil {
		return 0, 0, err
	}
	if err := jsonio.WriteArray(enhancementsPath, enhancements, *config.GlobalConfig.Pretty); err != nil {
		return 0, 0, err
	}
	fi, _ := os.Stat(itemsPath)
	efi, _ := os.Stat(enhancementsPath)

	if len(toDump) > 0 {
		_, dumpPath := b.outFilePath("dumped_items.json")
		if err := jsonio.WriteFile(dumpPath, toDump, *config.GlobalConfig.Pretty); err != nil {
			return 0, 0, err
		}
		dfi, _ := os.Stat(dumpPath)
		b.logf(fmt.Sprintf("dumped %d items to %s (%d bytes)", len(toDump), dumpPath, dfi.Size()))
	}

	return len(list), fi.Size() + efi.Size(), nil
}

// vkey is the strict identity that reissued copies of one item share (the bound
// reward/season/box duplicates the game mints under new ids). Name alone would
// conflate genuinely different items — 1,800+ same-name groups differ in icon or
// grade — so the key includes itemType, category, grade and icon. It also includes
// the equip slot: a Pearl-shop appearance costume ("Costume: Armor") reuses the
// real gear's name and icon, but an item worn in a different slot is never a
// reissue of the combat piece, so the two must stay separate records.
type vkey struct{ name, typ, cat, grade, icon, slot string }

// assignVariants picks the canonical record in each multi-copy group and points the
// others at it via VariantOf, returning the number of copies linked. The canonical
// is the tradeable/market item when one exists (its stat curve is the live version —
// some bound copies carry ±1-stat older snapshots), then highest NPC sell price,
// then lowest id.
func assignVariants(groups map[vkey][]*model.Item) (variants int) {
	rank := func(it *model.Item) (int64, int64) {
		score := int64(0)
		if it.Marketable {
			score = 2
		} else if it.SellPrice > 0 {
			score = 1
		}
		return score, it.SellPrice
	}
	for _, g := range groups {
		if len(g) < 2 {
			continue
		}
		canon := g[0]
		for _, it := range g[1:] {
			cs, cp := rank(canon)
			is, ip := rank(it)
			if is > cs || (is == cs && ip > cp) || (is == cs && ip == cp && it.ID < canon.ID) {
				canon = it
			}
		}
		for _, it := range g {
			if it != canon {
				it.VariantOf = model.ItemRef(canon.ID)
				variants++
			}
		}
	}
	return variants
}
