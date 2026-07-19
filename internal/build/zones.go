package build

import (
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
	"github.com/idevelopthings/bdo-data-extractor/src/urn"
	"github.com/idevelopthings/bdo-data-extractor/src/utils"
)

// buildZones decodes the Monster Zone Info table (dropuihuntinggroundinfo),
// resolving every referenced id to its name/icon inline, and registers zones.json.
// Skips if the table is absent; fails if present but corrupt.
func (b *Builder) buildZones() error {
	zoneData, ok, err := b.src.ReadIfExists("dropuihuntinggroundinfo.bss")
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	gs := b.gs
	isItem := func(v uint32) bool { _, ok := b.items[v]; return ok }
	zones, err := tables.DecodeZones(zoneData, isItem)
	if err != nil {
		return err
	}

	// tag colors come from dropuitaginfo; labels/descriptions/names from loc
	tagColor := map[uint32]model.TagInfo{}
	td, ok, err := b.src.ReadIfExists("dropuitaginfo.bss")
	if err != nil {
		return err
	}
	if ok {
		tags, err := tables.DecodeTags(td)
		if err != nil {
			return err
		}
		for _, t := range tags {
			tagColor[t.Key] = t
		}
	}
	// category icons (dropui*categoryinfo); names from loc.
	mainIcons := map[uint32]string{}
	mc, ok, err := b.src.ReadIfExists("dropuimaincategoryinfo.bss")
	if err != nil {
		return err
	}
	if ok {
		mainIcons = tables.DecodeMainCategoryIcons(mc)
	}
	subIcons := map[uint32]string{}
	sc, ok, err := b.src.ReadIfExists("dropuisubcategoryinfo.bss")
	if err != nil {
		return err
	}
	if ok {
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
			z.MainCategory.Name = gs.Territories[z.MainCategory.ID].Nation
			z.MainCategory.Icon = mainIcons[z.MainCategory.ID]
		}
		for j := range z.SubCategories {
			z.SubCategories[j].Name = gs.SubCatNames[z.SubCategories[j].ID]
			z.SubCategories[j].Icon = subIcons[z.SubCategories[j].ID]
		}
		for j := range z.Titles {
			t := gs.Titles[z.Titles[j].ID]
			z.Titles[j].Name = t.Name
			z.Titles[j].Desc = t.Description
		}
		for j := range z.Ecology {
			z.Ecology[j].Name = gs.Entities[z.Ecology[j].ID].Name
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
			tag := gs.Tags[k]
			z.Tags[j].Name = tag.Name
			z.Tags[j].Desc = tag.Description
			z.Tags[j].Color = c.Color
			z.Tags[j].FontColor = c.FontColor
		}
	}
	zp, err := b.addJSON("zones.json", zones)
	if err != nil {
		return err
	}
	b.logf(fmt.Sprintf("zones: %d (names resolved inline) -> %s", len(zones), zp))

	return nil
}
