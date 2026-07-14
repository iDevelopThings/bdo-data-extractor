package model

import (
	"github.com/idevelopthings/bdo-data-extractor/src/models"
	"github.com/idevelopthings/bdo-data-extractor/src/urn"
	"github.com/idevelopthings/bdo-data-extractor/src/utils"
)

// The Ref constructors below centralize the zero-value guard: a 0 id (or empty
// name) means "absent" and yields a nil ref, preserving the old omitempty
// behavior instead of pointing at a garbage URN like urn::item:0.

// ItemRef builds a reference to item id, or nil when id is 0.
func ItemRef(id uint32) *models.EntityRef[Item] {
	if id == 0 {
		return nil
	}
	return models.NewEntityRef[Item](urn.Item.New(id))
}

// ThemeRef builds a reference to knowledge theme key, or nil when key is 0.
func ThemeRef(key uint32) *models.EntityRef[KnowledgeTheme] {
	if key == 0 {
		return nil
	}
	return models.NewEntityRef[KnowledgeTheme](urn.Knowledge.New("theme", key))
}

// CaphrasRef builds a reference to caphras category key, or nil when key is 0.
func CaphrasRef(key int) *models.EntityRef[CaphrasCategory] {
	if key == 0 {
		return nil
	}
	return models.NewEntityRef[CaphrasCategory](urn.Caphras.New(key))
}

// CharacterRef builds a name-keyed reference (urn::character:<slug>) to the
// loc-6 entity a knowledge entry is about, or nil when the name is empty.
func CharacterRef(name string) *models.EntityRef[Character] {
	slug := utils.Slug(name)
	if slug == "" {
		return nil
	}
	return models.NewEntityRef[Character](urn.Character.New(slug))
}

// TerritoryRef builds a reference to territory index idx
// (urn::world:territory:<idx>). Territory indices are a 0-based enum (Balenos →
// 0), so unlike the id-keyed refs there is no zero guard — 0 is a real territory.
func TerritoryRef(idx int) *models.EntityRef[Territory] {
	return models.NewEntityRef[Territory](urn.World.New("territory", idx))
}

// NPCRef builds a reference to NPC id, or nil when id is 0.
func NPCRef(id uint32) *models.EntityRef[NPC] {
	if id == 0 {
		return nil
	}
	return models.NewEntityRef[NPC](urn.NPC.New(id))
}

// KnowledgeEntryRefList builds a list of knowledge-entry refs
// (urn::knowledge:entry:<key>) skipping 0 keys, or nil when the list is empty.
func KnowledgeEntryRefList(keys ...int) *models.EntityRefList[KnowledgeEntry] {
	l := models.EntityRefList[KnowledgeEntry]{}
	for _, key := range keys {
		if key != 0 {
			l.Add(urn.Knowledge.New("entry", key))
		}
	}
	if l.Len() == 0 {
		return nil
	}
	return &l
}

// ItemRefList builds a loot/ingredient list of item refs, skipping 0 ids.
func ItemRefList(ids ...uint32) models.EntityRefList[Item] {
	l := models.EntityRefList[Item]{}
	for _, id := range ids {
		if id != 0 {
			l.Add(urn.Item.New(id))
		}
	}
	return l
}

// WorldRegionRef builds a reference to map region key (urn::world:region:<key>), or
// nil when key is 0.
func WorldRegionRef(key uint32) *models.EntityRef[WorldRegion] {
	if key == 0 {
		return nil
	}
	return models.NewEntityRef[WorldRegion](urn.World.New("region", key))
}

// WorldNodeRef builds a reference to worldmap node key (urn::world:node:<key>), or
// nil when key is 0.
func WorldNodeRef(key uint32) *models.EntityRef[WorldNode] {
	if key == 0 {
		return nil
	}
	return models.NewEntityRef[WorldNode](urn.World.New("node", key))
}

// WorldNodeRefList builds a list of worldmap node refs, skipping 0 keys, or nil when
// the list is empty.
func WorldNodeRefList(keys ...uint32) *models.EntityRefList[WorldNode] {
	l := models.EntityRefList[WorldNode]{}
	for _, key := range keys {
		if key != 0 {
			l.Add(urn.World.New("node", key))
		}
	}
	if l.Len() == 0 {
		return nil
	}
	return &l
}

// NpcRefList builds a list of npc refs (urn::npc:<id>) skipping 0 ids, or nil when
// the list is empty (so a Character with no NPCs omits the field).
func NpcRefList(ids ...uint32) *models.EntityRefList[NPC] {
	l := models.EntityRefList[NPC]{}
	for _, id := range ids {
		if id != 0 {
			l.Add(urn.NPC.New(id))
		}
	}
	if l.Len() == 0 {
		return nil
	}
	return &l
}

// NewCharacter builds a knowledge-subject character keyed by name slug
// (urn::character:<slug>): its entity Kind (npc/monster/…) and the same-named
// entities that have an npcsimply record (npcIDs).
func NewCharacter(name string, kind CharacterKind, npcIDs ...uint32) Character {
	return Character{
		BaseFor: models.NewBaseForKey[Character](utils.Slug(name)),
		Name:    name,
		Kind:    kind,
		Npcs:    NpcRefList(npcIDs...),
	}
}
