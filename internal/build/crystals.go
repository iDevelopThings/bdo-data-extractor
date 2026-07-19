package build

import (
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

func (b *Builder) buildCrystalRules() error {
	groupData, err := b.src.Read("jewelgroupstaticstatus.bss")
	if err != nil {
		return err
	}
	rawGroups, err := tables.DecodeCrystalGroupRules(groupData)
	if err != nil {
		return err
	}
	groups := make([]model.CrystalGroupRule, 0, len(rawGroups))
	knownGroups := make(map[uint16]bool, len(rawGroups))
	for _, group := range rawGroups {
		localized, ok := b.gs.JewelGroups[uint32(group.Key)]
		if !ok || localized.Name == "" {
			return fmt.Errorf("jewelgroupstaticstatus: group %d is absent from loc table 121", group.Key)
		}
		if localized.Max != int(group.Max) {
			return fmt.Errorf("jewelgroupstaticstatus: group %d max is %d, loc table 121 says %d", group.Key, group.Max, localized.Max)
		}
		knownGroups[group.Key] = true
		groups = append(groups, model.CrystalGroupRule{
			Key:        uint32(group.Key),
			Name:       localized.Name,
			SourceName: group.SourceName,
			Max:        int(group.Max),
		})
	}

	slotData, err := b.src.Read("jewelspecialslotsgroupstaticstatus.bss")
	if err != nil {
		return err
	}
	rawSlots, err := tables.DecodeCrystalSpecialSlotRules(slotData)
	if err != nil {
		return err
	}
	slots := make([]model.CrystalSpecialSlotRule, 0, len(rawSlots))
	for _, slot := range rawSlots {
		if !slot.Slot.Valid() {
			return fmt.Errorf("jewelspecialslotsgroupstaticstatus: unknown special slot %d", slot.Slot)
		}
		allowed := make([]uint32, len(slot.AllowedGroups))
		for i, group := range slot.AllowedGroups {
			if !knownGroups[group] {
				return fmt.Errorf("jewelspecialslotsgroupstaticstatus: slot %s references missing group %d", slot.Slot, group)
			}
			allowed[i] = uint32(group)
		}
		slots = append(slots, model.CrystalSpecialSlotRule{Slot: slot.Slot, AllowedGroups: allowed})
	}

	path, err := b.addJSON("crystal_rules.json", model.CrystalRules{Groups: groups, SpecialSlots: slots})
	if err != nil {
		return err
	}
	b.logf(fmt.Sprintf("crystals: %d transfusion groups, %d special-slot restrictions -> %s", len(groups), len(slots), path))
	return nil
}
