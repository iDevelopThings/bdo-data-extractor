package build

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dgravesa/go-parallel/parallel"
	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
	"github.com/idevelopthings/bdo-data-extractor/src/models"
	"github.com/idevelopthings/bdo-data-extractor/src/recipe"
	"github.com/idevelopthings/bdo-data-extractor/src/utils"
)

// buildRecipes writes the recipes collected by scanItemInfo (during buildItems).
func (b *Builder) buildRecipes() error {
	// re-orient imperial-delivery recipes so the SpecialGoods delivery box is the
	// output (the XMLs list the relationship on both the good's and the box's page,
	// which otherwise yields a backwards "craft <good> from <its box>").
	sg := func(id uint32) bool { it := b.items[id]; return it != nil && it.Category == "SpecialGoods" }
	var orientedImperial int
	b.recipes, orientedImperial = recipe.NormalizeImperialRecipes(b.recipes, sg)
	b.logf(fmt.Sprintf("recipes: re-oriented %d imperial-delivery recipes to box-as-output", orientedImperial))
	// flag byproduct recipes (an item procs from a recipe that really crafts one
	// of its own ingredients) so consumers can keep them off the craft tree while
	// still showing "obtainable as a byproduct of X".
	b.logf(fmt.Sprintf("recipes: flagged %d byproduct recipes", recipe.MarkByproducts(b.recipes)))
	// stable sort by output only — preserves each item's in-XML recipe order, so
	// the game's primary recipe stays first (e.g. HEAT before the GRIND extractions).
	sort.SliceStable(b.recipes, func(i, j int) bool { return b.recipes[i].Output.ID() < b.recipes[j].Output.ID() })
	// assign each recipe its identity: urn::recipe:<outputId>:<index>, index being
	// its position among its output's recipes in this (written) order.
	recipeIdx := map[uint32]uint32{}
	for i := range b.recipes {
		out := b.recipes[i].Output.ID()
		b.recipes[i].BaseFor = models.NewBaseForKey[model.Recipe](out, recipeIdx[out])
		recipeIdx[out]++
	}
	p, err := b.write("recipes.json", b.recipes)
	if err != nil {
		return err
	}
	b.logf(fmt.Sprintf("recipes: %d (from per-item XMLs) -> %s", len(b.recipes), p))

	return nil
}

// scanItemInfo reads every localized per-item info XML once
// (ui_html/xml/<lang>/<id>.xml — these carry ingredient <count>; the non-localized
// originals don't, and the localized set has far more items), collecting the
// recipes that produce each item into b.recipes and attaching the item's
// acquisition (vendors / gather sources / nodes) onto the item. Runs during
// buildItems so items.json includes acquisition.
func (b *Builder) scanItemInfo() {
	krNpc := b.krNpcIndex()

	type acq struct{ vendors, gather, nodes []string }
	info := map[uint32]*acq{}
	get := func(id uint32) *acq {
		m := info[id]
		if m == nil {
			m = &acq{}
			info[id] = m
		}

		return m
	}
	enHasRecipe := map[uint32]bool{}

	// Pass 1: the localized (lang) XMLs — English names + ingredient counts. Both
	// passes enumerate only their ui_html/xml subtree (not the whole archive index)
	// and parse the XMLs concurrently, then reduce serially in file order.
	langFiles := b.src.Index.FilesUnder("ui_html/xml/" + strings.ToLower(b.lang))
	for _, ii := range b.parseItemInfos(langFiles, func(p string) bool { return isItemRecipeXML(p, b.lang) }) {
		if ii == nil {
			continue
		}
		b.recipes = append(b.recipes, ii.Recipes...)
		if len(ii.Recipes) > 0 {
			enHasRecipe[ii.Key] = true
		}
		m := get(ii.Key)
		m.vendors = appendAllUnique(m.vendors, ii.Vendors)
		m.gather = appendAllUnique(m.gather, ii.GatheredFrom)
		m.nodes = appendAllUnique(m.nodes, ii.GatherNodes)
	}

	// Pass 2: the base (Korean) XMLs — broader item coverage and often richer shop
	// data (502 base-only items have a vendor list the localized set lacks, e.g.
	// Sugar). Vendor names are Korean → resolve to the English NPC via npcsimply.
	// Recipes are only taken for items the localized set has none for (they lack
	// ingredient counts).
	baseFiles := b.src.Index.FilesUnder("ui_html/xml")
	for _, ii := range b.parseItemInfos(baseFiles, isBaseItemXML) {
		if ii == nil {
			continue
		}
		if !enHasRecipe[ii.Key] {
			b.recipes = append(b.recipes, ii.Recipes...)
		}
		m := get(ii.Key)
		// The base set is mixed-language: resolve Korean vendor names to the English
		// NPC; use already-English names as-is.
		for _, v := range ii.Vendors {
			if !utils.HasKorean(v) {
				m.vendors = utils.AppendUnique(m.vendors, v)

				continue
			}
			if id, ok := krNpc[v]; ok {
				if en := b.gs.EntityNames[id]; en != "" {
					m.vendors = utils.AppendUnique(m.vendors, en)
				}
			}
		}
	}

	for id, m := range info {
		it := b.items[id]
		if it == nil {
			continue
		}
		sort.Strings(m.vendors)
		sort.Strings(m.gather)
		sort.Strings(m.nodes)
		it.Vendors, it.GatheredFrom, it.GatherNodes = m.vendors, m.gather, m.nodes
	}
}

// parseItemInfos decodes the item-info XML of every file index that passes keep,
// concurrently (Archive.Content is concurrency-safe and ParseItemInfo is pure),
// returning results in the same order as indices — nil where the file is filtered,
// unreadable, or not item info. Parsing dominates scanItemInfo; the caller's reduce
// stays serial and in order, so the merged output is unchanged.
func (b *Builder) parseItemInfos(indices []int, keep func(path string) bool) []*tables.ItemInfo {
	out := make([]*tables.ItemInfo, len(indices))
	parallel.For(len(indices), func(k, _ int) {
		idx := indices[k]
		if !keep(b.src.Index.Path(idx)) {
			return
		}
		data, err := b.src.Archive.Content(b.src.Index.Files[idx])
		if err != nil {
			return
		}
		out[k] = tables.ParseItemInfo(data, b.houseName)
	})
	return out
}

// krNpcIndex maps an NPC's in-client Korean name to its id (from npcsimply, before
// the English-name override), so Korean vendor names in the base item XMLs can be
// resolved to the English NPC. First id wins on duplicate names (same name → same
// English name).
func (b *Builder) krNpcIndex() map[string]uint32 {
	out := map[string]uint32{}
	data, err := b.src.Read("npcsimply.bss")
	if err != nil {
		return out
	}
	npcs, err := tables.DecodeNPCs(data)
	if err != nil {
		return out
	}
	for _, n := range npcs {
		if n.Name != "" {
			if _, ok := out[n.Name]; !ok {
				out[n.Name] = n.ID
			}
		}
	}

	return out
}

// appendAllUnique unions src into dst, preserving order and skipping duplicates.
func appendAllUnique(dst, src []string) []string {
	for _, s := range src {
		dst = utils.AppendUnique(dst, s)
	}

	return dst
}

// houseName resolves a House Crafting workshop type to its localized name
// (loc table 123, e.g. 8 -> "Jeweler").
func (b *Builder) houseName(houseType int) string {
	return b.gs.HouseNames[uint32(houseType)]
}

// flagGathered marks raw/gathered materials from the crafting-UI palette
// (itemmaking.xml). A candidate that actually has a real production recipe is NOT a
// gathered material — some processed items are mis-listed under the gather palette
// (e.g. Flax Fabric appears under <collect> but is Ground from Flax Thread) — so
// those are skipped. Returns how many items were flagged. Runs after scanItemInfo
// so b.recipes is populated.
func (b *Builder) flagGathered() int {
	hasRealRecipe := map[uint32]bool{}
	for i := range b.recipes {
		if !recipe.IsExtraction(b.recipes[i]) {
			hasRealRecipe[b.recipes[i].Output.ID()] = true
		}
	}

	langDir := "ui_html/xml/" + strings.ToLower(b.lang)
	makingPath := langDir + "/itemmaking.xml"
	for _, i := range b.src.Index.FilesUnder(langDir) {
		if !strings.HasSuffix(strings.ToLower(b.src.Index.Path(i)), makingPath) {
			continue
		}
		data, err := b.src.Archive.Content(b.src.Index.Files[i])
		if err != nil {
			return 0
		}
		n := 0
		for _, id := range tables.ParseGatheredItems(data) {
			if it := b.items[id]; it != nil && !hasRealRecipe[id] {
				it.Gathered = true
				n++
			}
		}
		return n
	}

	return 0
}

// fillQuest resolves a QuestRef's "group-index" id to its loc texts.
func (b *Builder) fillQuest(q *model.QuestRef) {
	var group, index uint32
	if _, err := fmt.Sscanf(q.ID, "%d-%d", &group, &index); err != nil {
		return
	}
	qt := b.gs.Quests[group][index]
	q.Name = qt.Name
	q.Desc = qt.Desc
	q.Giver = qt.Giver
	q.Objective = qt.Objective
}

// isItemXML reports whether p is a per-item info XML named for its item id —
// prefix + <digits>.xml with no further subdirectory below prefix.
func isItemXML(p, prefix string) bool {
	p = strings.ToLower(p)
	i := strings.Index(p, prefix)
	if i < 0 {
		return false
	}
	base := p[i+len(prefix):]
	if strings.Contains(base, "/") { // a deeper subdir, not a leaf id.xml
		return false
	}
	name := strings.TrimSuffix(base, ".xml")
	if name == base || name == "" {
		return false
	}
	for _, c := range name {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// isBaseItemXML matches the non-localized (Korean base) per-item XMLs directly
// under ui_html/xml/ (not in a lang subdir).
func isBaseItemXML(p string) bool {
	return isItemXML(p, "ui_html/xml/")
}

// isItemRecipeXML matches the localized per-item XMLs ui_html/xml/<lang>/<id>.xml.
func isItemRecipeXML(p, lang string) bool {
	return isItemXML(p, "ui_html/xml/"+strings.ToLower(lang)+"/")
}
