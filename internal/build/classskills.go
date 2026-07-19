package build

import (
	"fmt"
	"sort"

	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

func (b *Builder) buildClassSkills(buffs map[uint16]tables.Buff, effects map[uint32]tables.SkillEffect) error {
	groupData, err := b.src.Read("skillgroup.bss")
	if err != nil {
		return err
	}
	groups, err := tables.DecodeSkillGroups(groupData)
	if err != nil {
		return err
	}
	typeOffset, err := b.src.Read("skilltypeoffset.dbss")
	if err != nil {
		return err
	}
	typeData, err := b.src.Read("skilltype.dbss")
	if err != nil {
		return err
	}
	types, err := tables.DecodeSkillTypeHeaders(typeOffset, typeData)
	if err != nil {
		return err
	}

	var decodedTrees []tables.SkillTree
	var treeMetadata []model.ClassSkillTreeMetadata
	for _, source := range []struct {
		file string
		kind tables.SkillTreeKind
	}{
		{file: "ui_skillgroup_combat.bss", kind: tables.SkillTreeCombat},
		{file: "ui_skillgroup_awakening.bss", kind: tables.SkillTreeAwakening},
	} {
		data, readErr := b.src.Read(source.file)
		if readErr != nil {
			return readErr
		}
		table, decodeErr := tables.DecodeSkillTrees(data, source.kind)
		if decodeErr != nil {
			return decodeErr
		}
		decodedTrees = append(decodedTrees, table.Trees...)
		treeMetadata = append(treeMetadata, model.ClassSkillTreeMetadata{
			Kind:               string(source.kind),
			SubGroupStringKeys: table.SubGroupStringKeys,
			UnknownFooter:      table.UnknownFooter,
		})
	}

	classesByGroup := make(map[uint16]map[model.CharacterClassType]bool)
	trees := make([]model.ClassSkillTree, 0, len(decodedTrees))
	for _, tree := range decodedTrees {
		if !tree.ClassType.IsPlayable() {
			continue
		}
		cells := make([]model.ClassSkillTreeCell, len(tree.Cells))
		for i, cell := range tree.Cells {
			cells[i] = model.ClassSkillTreeCell{
				X:         cell.X,
				Y:         cell.Y,
				Types:     cell.Types,
				Group:     cell.Group,
				SubGroup:  cell.SubGroup,
				Unknown16: cell.Unknown16,
			}
			if !cell.HasSkillGroup() || cell.Group == 0 {
				continue
			}
			if _, ok := groups[cell.Group]; !ok {
				return fmt.Errorf("class skill tree: class %d %s cell %d,%d references missing group %d", tree.ClassType, tree.Kind, cell.X, cell.Y, cell.Group)
			}
			if classesByGroup[cell.Group] == nil {
				classesByGroup[cell.Group] = make(map[model.CharacterClassType]bool)
			}
			classesByGroup[cell.Group][tree.ClassType] = true
		}
		trees = append(trees, model.ClassSkillTree{
			ClassType: tree.ClassType,
			Kind:      string(tree.Kind),
			Width:     tree.Width,
			Height:    tree.Height,
			Cells:     cells,
		})
	}

	groupKeys := make([]int, 0, len(classesByGroup))
	for key := range classesByGroup {
		groupKeys = append(groupKeys, int(key))
	}
	sort.Ints(groupKeys)
	resolvedGroups := make([]model.ClassSkillGroup, 0, len(groupKeys))
	passiveGroups := 0
	for _, rawKey := range groupKeys {
		key := uint16(rawKey)
		group := groups[key]
		classes := make([]model.CharacterClassType, 0, len(classesByGroup[key]))
		for classType := range classesByGroup[key] {
			classes = append(classes, classType)
		}
		sort.Slice(classes, func(i, j int) bool {
			return classes[i] < classes[j]
		})

		resolved := model.ClassSkillGroup{Key: key, Classes: classes}
		isPassive := false
		for rank, skillKey := range group.SkillKeys {
			if skillKey == 0 {
				continue
			}
			header, ok := types[skillKey]
			if !ok {
				return fmt.Errorf("class skill group %d rank %d: skilltype key %#x is missing", key, rank, skillKey)
			}
			skillNo := uint16(skillKey >> 16)
			localized := b.gs.SkillTexts[uint32(skillNo)]
			resolvedRank := model.ClassSkillRank{
				Rank:            rank,
				SkillKey:        skillKey,
				SkillNo:         skillNo,
				SkillLevel:      uint16(skillKey),
				Kind:            header.Kind,
				Name:            localized.Name,
				Description:     localized.Description,
				SourceName:      header.SourceName,
				SourceGroupName: header.SourceGroupName,
			}
			if resolved.Name == "" && localized.Name != "" {
				resolved.Name = localized.Name
			}
			if header.Kind == model.SkillKindPassive {
				isPassive = true
				if effect, exists := effects[skillKey]; exists {
					resolvedRank.Effects = b.buildEffects(buffs, effect)
				}
			}
			resolved.Ranks = append(resolved.Ranks, resolvedRank)
		}
		if isPassive {
			passiveGroups++
		}
		resolvedGroups = append(resolvedGroups, resolved)
	}

	path, err := b.addJSON("class_skills.json", model.ClassSkillData{Groups: resolvedGroups, Trees: trees, TreeMetadata: treeMetadata})
	if err != nil {
		return err
	}
	b.logf(fmt.Sprintf("class skills: %d groups (%d passive), %d class trees -> %s", len(resolvedGroups), passiveGroups, len(trees), path))
	return nil
}
