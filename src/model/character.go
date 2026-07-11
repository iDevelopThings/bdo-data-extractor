package model

import "github.com/idevelopthings/bdo-data-extractor/src/models"

// CharacterKind is the coarse entity kind of a knowledge subject, derived from the
// characterstatic.dbss model-path prefix. A typed string so Wails emits a TS union.
type CharacterKind string

const (
	CharacterKindNPC        CharacterKind = "npc"
	CharacterKindMonster    CharacterKind = "monster"
	CharacterKindCreature   CharacterKind = "creature"
	CharacterKindObject     CharacterKind = "object"
	CharacterKindGatherable CharacterKind = "gatherable"
	CharacterKindStructure  CharacterKind = "structure"
	CharacterKindMount      CharacterKind = "mount"
	CharacterKindCash       CharacterKind = "cash"
	CharacterKindSummon     CharacterKind = "summon"
	CharacterKindPC         CharacterKind = "pc"
)

// characterKindByNpcKind maps the clean low values of the characterstatic npcKind
// field (the game's semantic entity type) to a CharacterKind. This is the
// authoritative source where it applies — e.g. npcKind 8 = a statue/decorative
// object (Kzarka Statue), which the render model calls "monster/dummy". Combat
// entities carry higher flag bits instead (see CharacterStatic.NpcKind) and fall
// back to the render-model prefix.
var characterKindByNpcKind = map[uint32]CharacterKind{
	1: CharacterKindNPC,        // managers / peddler wagons (functional npcs)
	2: CharacterKindNPC,        // people
	3: CharacterKindObject,     // containers (boxes/eggs/supplies)
	4: CharacterKindGatherable, // trees / rocks / ore
	7: CharacterKindStructure,  // flags / forts / node objects
	8: CharacterKindObject,     // statues / vases / decorative objects
	9: CharacterKindStructure,  // fences / barricades / siege installations
}

// characterKindByModelPrefix maps a characterstatic model-path leading segment to a
// CharacterKind. A '/'-prefixed model path whose prefix is absent here is an
// unrecognized kind — the extractor hard-panics on it so a new content type can't
// be silently misclassified; add the prefix here when that fires.
var characterKindByModelPrefix = map[string]CharacterKind{
	"npc":       CharacterKindNPC,
	"monster":   CharacterKindMonster,
	"creature":  CharacterKindCreature,
	"object":    CharacterKindObject,
	"riding":    CharacterKindMount,
	"cash":      CharacterKindCash,
	"summon":    CharacterKindSummon,
	"pc_summon": CharacterKindSummon,
	"pc":        CharacterKindPC,
	// Infinite Defense zone/structure entities (e.g. "Tungrad Ruins") — location
	// encounters, closest to an object among the kinds.
	"infinitydefence": CharacterKindObject,
}

// characterKindPriority orders kinds when a name maps to entities of several kinds
// (higher wins) — a knowledge subject is classified by its most character-like id.
var characterKindPriority = map[CharacterKind]int{
	CharacterKindNPC:        9,
	CharacterKindCreature:   8,
	CharacterKindMonster:    7,
	CharacterKindGatherable: 6,
	CharacterKindStructure:  5,
	CharacterKindObject:     4,
	CharacterKindMount:      3,
	CharacterKindSummon:     2,
	CharacterKindCash:       1,
	CharacterKindPC:         1,
}

// CharacterKindFromNpcKind resolves the characterstatic npcKind field to a
// CharacterKind for its clean low values; ok is false for 0 (unread) or the higher
// flag-bitfield values (combat entities), which fall back to CharacterKindFromModelPrefix.
func CharacterKindFromNpcKind(npcKind uint32) (kind CharacterKind, ok bool) {
	k, ok := characterKindByNpcKind[npcKind]
	return k, ok
}

// CharacterKindFromModelPrefix resolves a characterstatic model-path prefix to a
// CharacterKind; ok is false when the prefix isn't a known kind.
func CharacterKindFromModelPrefix(prefix string) (kind CharacterKind, ok bool) {
	k, ok := characterKindByModelPrefix[prefix]
	return k, ok
}

// CharacterKindPriority ranks a kind for the multi-entity tiebreak (0 = unset).
func CharacterKindPriority(k CharacterKind) int { return characterKindPriority[k] }

// CharacterEntity is one characterstatic.dbss record behind a subject (a subject
// aggregates same-named loc-6 entities). It carries everything we currently read
// from the record, including still-unidentified fields, so nothing is lost.
type CharacterEntity struct {
	ID uint32 `json:"id"`
	// Kind is this entity's kind (from NpcKind where clean, else the Model prefix).
	Kind CharacterKind `json:"kind,omitempty"`
	// NpcKind is the raw characterstatic semantic-type bitfield (low values are the
	// clean kinds; higher values carry undecoded flags for combat entities). 0 when
	// the record's id didn't land cleanly.
	NpcKind uint32 `json:"npcKind,omitempty"`
	// Model is the asset model path (render classification / kind fallback).
	Model string `json:"model,omitempty"`
	// Scripts are the entity's interaction scripts verbatim (e.g. "getknowledge(1);").
	Scripts []string `json:"scripts,omitempty"`
	// Card is the knowledge card key from a getknowledge(N) script (id-based
	// entity→card link), 0 if none.
	Card uint32 `json:"card,omitempty"`
	// Fields are the still-unidentified structured u32s that follow npcKind, captured
	// raw for future RE.
	Fields []uint32 `json:"fields,omitempty"`
}

// Character is a knowledge subject — the loc-table-6 entity(-ies) a knowledge card
// is about, keyed by name slug (urn::character:<slug>). loc table 6 mixes many
// entity kinds under one id space and has duplicate names, so a subject can't be a
// single NPC id; it's name-keyed and aggregates the same-named entities.
//
// Kind is the subject's entity kind (the most character-like of its entities'
// kinds; from the characterstatic npcKind field where clean, else the render model
// prefix) — empty when no entity carries a model (a place/abstract). Npcs are the
// same-named entities that also have an npcsimply record (spawns/dialogue) — so
// "Islin Bartali" carries npc 40001 while "Kzarka Statue" is a kind=object subject
// (npcKind 8) with no npcs. Entities is the full per-entity characterstatic data.
type Character struct {
	*models.BaseFor[Character]

	Name     string                     `json:"name,omitempty"`
	Kind     CharacterKind              `json:"kind,omitempty"`
	Npcs     *models.EntityRefList[NPC] `json:"npcs,omitempty"`
	Entities []CharacterEntity          `json:"entities,omitempty"`
}
