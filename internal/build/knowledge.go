package build

import (
	"fmt"
	"sort"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
	"github.com/idevelopthings/bdo-data-extractor/src/utils"
)

// buildKnowledge decodes the mentaltheme tree + mentalcard entries (names from
// loc tables 9/34), links them to items/NPCs by name, and writes knowledge.json.
// Skips if absent.
func (b *Builder) buildKnowledge() error {
	themeData, err := b.src.Read("mentaltheme.dbss")
	if err != nil {
		return nil
	}
	themeOff, _ := b.src.Read("mentalthemeoffset.dbss")
	themes := tables.DecodeKnowledgeThemes(themeOff, themeData, b.gs.ThemeNames)

	var entries []model.KnowledgeEntry
	if cardData, err := b.src.Read("mentalcard.dbss"); err == nil {
		cardOff, _ := b.src.Read("mentalcardoffset.dbss")
		entries = tables.DecodeKnowledgeEntries(cardOff, cardData, b.gs.CardNames, b.gs.CardDescs, b.gs.CardAcquire)
	}
	itemLinks := b.linkKnowledge(themes, entries)
	characters, charLinks := b.buildCharacters(entries)

	kp, err := b.write("knowledge.json", map[string]any{"themes": themes, "entries": entries})
	if err != nil {
		return err
	}
	b.logf(
		fmt.Sprintf(
			"knowledge: %d themes, %d entries (%d item links, %d character links) -> %s",
			len(themes), len(entries), itemLinks, charLinks, kp,
		),
	)

	cp, err := b.write("characters.json", characters)
	if err != nil {
		return err
	}
	b.logf(fmt.Sprintf("characters: %d subjects (kind from npcKind/model + per-entity scripts/fields, from characterstatic/loc6/npcsimply) -> %s", len(characters), cp))

	return nil
}

// linkKnowledge wires the "You can learn about …" item onto the themes/entries by
// matching names, and returns the item-link count. A theme matches a whole-category
// item; an entry matches a single item. (The entry's subject/character link is
// handled separately by buildCharacters.)
func (b *Builder) linkKnowledge(themes []model.KnowledgeTheme, entries []model.KnowledgeEntry) (itemLinks int) {
	// name -> knowledge item id, for the "You can learn about …" items. On a name
	// collision keep the lowest id, so the link is deterministic across runs (map
	// iteration order isn't).
	kItem := map[string]uint32{}
	for id, it := range b.items {
		if it.ItemType == "Skill" && it.Name != "" {
			l := strings.ToLower(it.Name)
			if cur, ok := kItem[l]; !ok || id < cur {
				kItem[l] = id
			}
		}
	}
	for i := range themes {
		if id := kItem[strings.ToLower(themes[i].Name)]; id != 0 {
			themes[i].Item = model.ItemRef(id)
			itemLinks++
		}
	}
	for i := range entries {
		if id := kItem[strings.ToLower(entries[i].Name)]; id != 0 {
			entries[i].Item = model.ItemRef(id)
			itemLinks++
		}
	}

	return itemLinks
}

// buildCharacters resolves each knowledge entry's subject to a Character record —
// the loc-6 entity/entities that share the entry's name — and returns the deduped
// character dataset plus the number of entries linked. It sets entries[i].Character
// in place. A character's Kind comes from the characterstatic.dbss model path of
// its entities (npc/monster/creature/object/…); its Npcs are the same-named
// entities that have an npcsimply record. Duplicate names are aggregated (no longer
// dropped), so e.g. "Islin Bartali" links to npcs [40001, 59516] and "Kzarka
// Statue" is a kind=monster record — the 1289 non-NPC subjects become real records.
func (b *Builder) buildCharacters(entries []model.KnowledgeEntry) ([]model.Character, int) {
	var static map[uint32]tables.CharacterStatic
	if off, err := b.src.Read("characterstaticoffset.dbss"); err == nil {
		if dat, derr := b.src.Read("characterstatic.dbss"); derr == nil {
			static = tables.DecodeCharacterStatic(off, dat)
		}
	}
	idsByName := map[string][]uint32{}
	for id, nm := range b.gs.EntityNames {
		l := strings.ToLower(nm)
		idsByName[l] = append(idsByName[l], id)
	}
	isNpc := make(map[uint32]bool, len(b.npcs))
	for _, n := range b.npcs {
		isNpc[n.ID] = true
	}

	chars := map[string]*model.Character{}
	links := 0
	for i := range entries {
		nm := entries[i].Name
		ids := idsByName[strings.ToLower(nm)]
		if nm == "" || len(ids) == 0 {
			continue
		}
		entries[i].Character = model.CharacterRef(nm)
		links++

		slug := utils.Slug(nm)
		if _, ok := chars[slug]; ok {
			continue
		}
		sort.Slice(ids, func(a, c int) bool { return ids[a] < ids[c] })
		var npcIDs []uint32
		for _, id := range ids {
			if isNpc[id] {
				npcIDs = append(npcIDs, id)
			}
		}
		ents, kind, unknown := characterEntities(ids, static)
		if kind == "" && unknown != "" {
			panic(fmt.Sprintf(
				"buildCharacters: knowledge subject %q (entities %v) has an unmapped characterstatic model kind %q — add it to model.characterKindByModelPrefix",
				nm, ids, unknown,
			))
		}
		ch := model.NewCharacter(nm, kind, npcIDs...)
		ch.Entities = ents
		chars[slug] = &ch
	}

	out := make([]model.Character, 0, len(chars))
	for _, ch := range chars {
		out = append(out, *ch)
	}
	sort.Slice(out, func(a, c int) bool { return out[a].Name < out[c].Name })
	return out, links
}

// characterEntities builds the per-entity records for a set of same-named loc-6
// entities from their characterstatic data, and picks the subject's aggregate
// CharacterKind (the most character-like of the entities' kinds). Each entity's kind
// comes from its npcKind field where that's a clean value, else the render model-path
// prefix. It returns best=="" for a subject whose entities carry no kind at all (a
// place/abstract), and unknownPrefix set (with best=="") when an entity's model
// prefix isn't a known kind and nothing else resolved — the caller hard-panics on
// that so the kind taxonomy stays exhaustive as content is added.
func characterEntities(ids []uint32, static map[uint32]tables.CharacterStatic) (ents []model.CharacterEntity, best model.CharacterKind, unknownPrefix string) {
	bestRank := 0
	for _, id := range ids {
		cs := static[id]
		ek, ok := model.CharacterKindFromNpcKind(cs.NpcKind)
		if !ok {
			if pre := tables.ModelPrefix(cs.Model); pre != "" {
				if k, kok := model.CharacterKindFromModelPrefix(pre); kok {
					ek = k
				} else {
					unknownPrefix = pre
				}
			}
		}
		ents = append(ents, model.CharacterEntity{
			ID:      id,
			Kind:    ek,
			NpcKind: cs.NpcKind,
			Model:   cs.Model,
			Scripts: cs.Scripts,
			Card:    cs.Card,
			Fields:  cs.Fields,
		})
		if ek != "" {
			if r := model.CharacterKindPriority(ek); r > bestRank {
				best, bestRank = ek, r
			}
		}
	}
	if best != "" {
		unknownPrefix = "" // a known kind resolved; don't panic over a sibling unknown
	}
	return ents, best, unknownPrefix
}
