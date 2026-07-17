package build

import (
	"fmt"
	"sort"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/loc"
	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

var itemSetByMarkerPrefix = map[string]uint32{
	"ANCIENT_":              57337, // Slumbering Origin & Edana's Defense Gear Effect
	"EDANA_":                57337, // Slumbering Origin & Edana's Defense Gear Effect
	"BLACKSTAR_":            52494, // Blackstar Armor Effect
	"DEBOREKA_":             58080, // Deboreka Accessory Effect
	"TUNGRAD_":              58454, // Tungrad Accessory Effect
	"GBEAR_":                47639, // Bear Necessities Set Effect
	"SET_DECORATE_Training": 57482, // Venia Riding Set Effect
}

var itemSetByDSLFunc = map[string]uint32{
	"ACCSET_1GRADE_LIFE_EXP_POINT_ADD": 57991, // Loggia Accessory Set
	"ACCSET_2GRADE_LIFE_EXP_POINT_ADD": 57992, // Geranoa Accessory Set
	"ACCSET_5GRADE_LIFE_EXP_POINT_ADD": 57993, // Manos and Preonne Accessory Set
	"ACCSET_6GRADE_LIFE_EXP_POINT_ADD": 57551, // Preonne Accessory Set
}

var explicitItemSetMembers = map[uint32][]uint32{
	51134: { // Boss Armor Set
		11013, // Giath's Helmet
		11014, // Red Nose's Armor
		11015, // Bheg's Gloves
		11016, // Muskan's Shoes
		11017, // Dim Tree Spirit's Armor
		11101, // Griffon's Helmet
		11102, // Leebur's Gloves
		11103, // Urugon's Shoes
	},
}

func (b *Builder) buildItemSets() error {
	offsetData, err := b.src.Read("skillpieceoffset.dbss")
	if err != nil {
		return err
	}
	data, err := b.src.Read("skillpiece.dbss")
	if err != nil {
		return err
	}
	sets, err := tables.DecodeItemSets(offsetData, data)
	if err != nil {
		return err
	}
	localizedFields := localizeItemSets(sets, b.gs.ItemSetTexts)
	b.itemSets = sets

	memberships := make(map[uint32]map[uint32]model.ItemSetMembershipSource)
	add := func(itemID, setKey uint32, source model.ItemSetMembershipSource) {
		bySet := memberships[itemID]
		if bySet == nil {
			bySet = make(map[uint32]model.ItemSetMembershipSource)
			memberships[itemID] = bySet
		}
		bySet[setKey] = source
	}

	for itemID, enhancement := range b.enhancements {
		groups := itemSetEffectGroups(enhancement)
		for _, group := range groups {
			if setKey := itemSetForMarker(group.Marker); setKey != 0 {
				add(itemID, setKey, model.ItemSetMembershipDSL)
			}
		}
		for _, level := range enhancement.Levels {
			for _, group := range level.Effects {
				for _, stat := range group.Stats {
					if stat.EffectDsl == nil {
						continue
					}
					if setKey := itemSetByDSLFunc[stat.Func]; setKey != 0 {
						add(itemID, setKey, model.ItemSetMembershipDSL)
					}
				}
			}
		}
	}
	for setKey, itemIDs := range explicitItemSetMembers {
		for _, itemID := range itemIDs {
			if b.items[itemID] == nil {
				return fmt.Errorf("item set %d: explicit member item %d is missing", setKey, itemID)
			}
			add(itemID, setKey, model.ItemSetMembershipExplicit)
		}
	}
	// Reissued bound/reward copies point directly at their canonical item.
	// Carry the canonical set membership onto those records as well.
	for itemID, item := range b.items {
		if item.VariantOf == nil {
			continue
		}
		for setKey, source := range memberships[item.VariantOf.ID()] {
			add(itemID, setKey, source)
		}
	}

	setsByKey := make(map[uint32]*model.ItemSet, len(b.itemSets))
	for i := range b.itemSets {
		setsByKey[b.itemSets[i].SkillNo] = &b.itemSets[i]
	}
	for _, setKey := range itemSetByDSLFunc {
		if setsByKey[setKey] == nil {
			return fmt.Errorf("item set %d referenced by DSL is missing from skillpiece", setKey)
		}
	}
	for _, setKey := range itemSetByMarkerPrefix {
		if setsByKey[setKey] == nil {
			return fmt.Errorf("item set %d referenced by a DSL marker is missing from skillpiece", setKey)
		}
	}
	for setKey := range explicitItemSetMembers {
		if setsByKey[setKey] == nil {
			return fmt.Errorf("explicit item set %d is missing from skillpiece", setKey)
		}
	}

	itemIDs := make([]uint32, 0, len(memberships))
	for itemID := range memberships {
		itemIDs = append(itemIDs, itemID)
	}
	sort.Slice(itemIDs, func(i, j int) bool {
		return itemIDs[i] < itemIDs[j]
	})

	setItems := make(map[uint32][]uint32)
	setSources := make(map[uint32]map[model.ItemSetMembershipSource]bool)
	for _, itemID := range itemIDs {
		setKeys := make([]uint32, 0, len(memberships[itemID]))
		for setKey := range memberships[itemID] {
			setKeys = append(setKeys, setKey)
		}
		sort.Slice(setKeys, func(i, j int) bool {
			return setKeys[i] < setKeys[j]
		})
		b.items[itemID].ItemSets = model.ItemSetRefList(setKeys...)
		for _, setKey := range setKeys {
			setItems[setKey] = append(setItems[setKey], itemID)
			if setSources[setKey] == nil {
				setSources[setKey] = make(map[model.ItemSetMembershipSource]bool)
			}
			setSources[setKey][memberships[itemID][setKey]] = true
		}
	}

	for setKey, itemIDs := range setItems {
		set := setsByKey[setKey]
		items := model.ItemRefList(itemIDs...)
		set.Items = &items
		for _, source := range []model.ItemSetMembershipSource{model.ItemSetMembershipDSL, model.ItemSetMembershipExplicit} {
			if setSources[setKey][source] {
				set.MembershipSources = append(set.MembershipSources, source)
			}
		}
	}

	name, err := b.write("item_sets.json", b.itemSets)
	if err != nil {
		return err
	}
	b.logf(fmt.Sprintf("skillpiece: %d set definitions, %d linked sets, %d localized fields, %d linked items -> %s", len(b.itemSets), len(setItems), localizedFields, len(memberships), name))
	return nil
}

func itemSetEffectGroups(enhancement *model.Enhancement) []model.EffectGroup {
	groups := make([]model.EffectGroup, 0)
	for _, level := range enhancement.Levels {
		for _, group := range level.Effects {
			if !isItemSetMarker(group.Marker) {
				continue
			}
			groups = append(groups, group)
		}
	}
	return groups
}

func isItemSetMarker(marker string) bool {
	return strings.Contains(marker, "SET_EFFECT") ||
		strings.Contains(marker, "WEAR_EFFECT") ||
		strings.Contains(marker, "CASH_UP") ||
		strings.HasPrefix(marker, "SET_DECORATE_") ||
		marker == "ALCHEMY_4_SET" || marker == "COLLECT_4_SET"
}

func itemSetForMarker(marker string) uint32 {
	for prefix, setKey := range itemSetByMarkerPrefix {
		if strings.HasPrefix(marker, prefix) {
			return setKey
		}
	}
	return 0
}

func localizeItemSets(sets []model.ItemSet, strings loc.Table) int {
	localized := 0
	for i := range sets {
		set := &sets[i]
		for j := range set.Bonuses {
			bonus := &set.Bonuses[j]
			apply := uint32(bonus.Apply)
			if text, ok := strings.Lookup(set.SkillNo, apply); ok {
				bonus.Description = text
				localized++
			}
			if text, ok := strings.Lookup(set.SkillNo, 0x01000000|apply); ok {
				bonus.DescriptionTitle = text
				localized++
			}
			if text, ok := strings.Lookup(set.SkillNo, 0x02000000|apply); ok {
				bonus.GroupTitle = text
				localized++
			}
		}
	}
	return localized
}
