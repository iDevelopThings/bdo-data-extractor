package build

import (
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
	"github.com/idevelopthings/bdo-data-extractor/src/urn"
	"github.com/idevelopthings/bdo-data-extractor/src/utils"
)

// buildZones decodes the Monster Zone Info table (dropuihuntinggroundinfo),
// resolving every referenced id to its name/icon inline, and writes zones.json.
// Skips silently if the table is absent.
func (b *Builder) buildZones() error {
	zoneData, err := b.src.Read("dropuihuntinggroundinfo.bss")
	if err != nil {
		return nil
	}
	gs := b.gs
	isItem := func(v uint32) bool { _, ok := b.items[v]; return ok }
	zones := tables.DecodeZones(zoneData, isItem)

	// tag colors come from dropuitaginfo; labels/descriptions/names from loc
	tagColor := map[uint32]model.TagInfo{}
	if td, err := b.src.Read("dropuitaginfo.bss"); err == nil {
		for _, t := range tables.DecodeTags(td) {
			tagColor[t.Key] = t
		}
	}
	// category icons (dropui*categoryinfo); names from loc.
	mainIcons := map[uint32]string{}
	if mc, err := b.src.Read("dropuimaincategoryinfo.bss"); err == nil {
		mainIcons = tables.DecodeMainCategoryIcons(mc)
	}
	subIcons := map[uint32]string{}
	if sc, err := b.src.Read("dropuisubcategoryinfo.bss"); err == nil {
		subIcons = tables.DecodeSubCategoryIcons(sc)
	}
	// A zone's node key is a worldmap node key for 99 of the 105 zones; the rest point
	// at a key with no exploration record, so only those 99 get a resolvable ref.
	explorationNodes := map[uint32]bool{}
	if nodes, err := b.explorationTable(); err == nil {
		for i := range nodes {
			explorationNodes[uint32(nodes[i].Key)] = true
		}
	}

	// resolve every referenced id to its name inline on the zone (no sidecars).
	// loot stays as item ids -> items.json (the canonical item source).
	for i := range zones {
		z := &zones[i]
		if i < len(gs.ZoneNames) { // English zone/node name (loc table 116, record order)
			z.Name = gs.ZoneNames[i]
			if z.Node != nil {
				z.Node.Name = gs.ZoneNames[i]
			}
		}
		if z.Node != nil && explorationNodes[z.Node.Key] {
			z.Node.Node = model.WorldNodeRef(z.Node.Key)
		}
		if z.MainCategory != nil {
			z.MainCategory.Name = gs.MainCatNames[z.MainCategory.ID]
			z.MainCategory.Icon = mainIcons[z.MainCategory.ID]
		}
		for j := range z.SubCategories {
			z.SubCategories[j].Name = gs.SubCatNames[z.SubCategories[j].ID]
			z.SubCategories[j].Icon = subIcons[z.SubCategories[j].ID]
		}
		for j := range z.Titles {
			z.Titles[j].Name = gs.Titles[z.Titles[j].ID]
			z.Titles[j].Desc = gs.TitleDescs[z.Titles[j].ID]
		}
		for j := range z.Ecology {
			z.Ecology[j].Name = gs.EntityNames[z.Ecology[j].ID]
			if slug := utils.Slug(z.Ecology[j].Name); slug != "" {
				z.Ecology[j].URN = new(urn.Character.New(slug))
			}
		}
		for j := range z.Topography {
			z.Topography[j].Name = gs.Topography[z.Topography[j].ID]
			z.Topography[j].URN = new(urn.World.New("region", z.Topography[j].ID))
		}
		for j := range z.RecurringQuests {
			b.fillQuest(&z.RecurringQuests[j])
		}
		for j := range z.RegionQuests {
			b.fillQuest(&z.RegionQuests[j])
		}
		for j := range z.Tags {
			k := z.Tags[j].Key
			c := tagColor[k]
			z.Tags[j].Name = gs.Tags[k]
			z.Tags[j].Desc = gs.TagDescs[k]
			z.Tags[j].Color = c.Color
			z.Tags[j].FontColor = c.FontColor
		}
	}
	zp, err := b.write("zones.json", zones)
	if err != nil {
		return err
	}
	b.logf(fmt.Sprintf("zones: %d (names resolved inline) -> %s", len(zones), zp))

	return nil
}
