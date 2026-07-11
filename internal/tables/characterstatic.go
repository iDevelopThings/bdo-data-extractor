package tables

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
)

// CharacterStatic is what we read from one characterstatic.dbss entity record.
type CharacterStatic struct {
	// NpcKind is the game's semantic entity-type field. It's a bitfield: low values
	// (1,2,3,4,7,8,9,17) are clean kinds (npc/object/gatherable/structure/…, and
	// e.g. 8 = statue/decorative-object — questlog's npcKind), while combat entities
	// (monsters/bosses/pets/vehicles) carry higher flag bits (0x10001, 0x401, …)
	// that aren't fully decoded. 0 = not read (a ~10% minority of records whose id
	// didn't land cleanly; those fall back to the model kind).
	NpcKind uint32
	// Model is the asset model path (e.g. "npc/npc_named_stand", "monster/dummy_normal")
	// — the render classification, used as a kind fallback.
	Model string
	// Card is the knowledge card key from a getknowledge(N) interaction script on the
	// entity (the real id-based entity→card link), 0 if the entity has no such script.
	Card uint32
	// Scripts are the entity's (non-empty) interaction scripts verbatim, e.g.
	// "getknowledge(1);", "getoffwork();".
	Scripts []string
	// Fields are the still-unidentified structured u32s that follow npcKind up to the
	// model path — captured raw (nothing dropped) for future RE. Empty when the id
	// didn't land cleanly.
	Fields []uint32
}

// DecodeCharacterStatic decodes characterstatic.dbss into entity-id -> CharacterStatic.
// The offset companion is PABR + u32 count@4 + 10-byte index [u16 key, u32 off, u32
// size]; the key is the entity id (= loc table 6 id). Each record is a sequential
// field stream, read here with a bss.Cursor:
//
//	[8-byte header][u8 tag=0x15][ [i64 charCount][UTF-16] × 2 interaction scripts ]
//	[u8][u32 id][u32 npcKind][ … render/config fields, incl. the model path … ]
//
// Reading the two length-prefixed scripts lands deterministically on id+npcKind
// (validated id == key). A getknowledge(N) script yields the knowledge-card link;
// the model path is scanned from the record.
func DecodeCharacterStatic(offsetRaw, dataRaw []byte) map[uint32]CharacterStatic {
	if len(offsetRaw) < 8 {
		return nil
	}
	count := int(bss.U32(offsetRaw, 4))
	out := make(map[uint32]CharacterStatic, count)
	for i := 0; i < count; i++ {
		p := 8 + i*10
		if p+10 > len(offsetRaw) {
			break
		}
		key := uint32(bss.U16(offsetRaw, p))
		off := int(bss.U32(offsetRaw, p+2))
		size := int(bss.U32(offsetRaw, p+6))
		if key == 0 || off < 0 || size < 8 || off+size > len(dataRaw) {
			continue
		}
		rec := dataRaw[off : off+size]

		model, modelOff := modelPathAt(rec)
		cs := CharacterStatic{Model: model}
		c := bss.NewCursor(rec, 9, len(rec)) // start after the @8 tag byte
		for s := 0; s < 2; s++ {
			script := c.UTF16()
			if script != "" {
				cs.Scripts = append(cs.Scripts, script)
			}
			if card := getKnowledgeCard(script); card != 0 {
				cs.Card = card
			}
		}
		c.Skip(1)
		id := c.U32()
		npcKind := c.U32()
		if c.OK() && id == key { // scripts parsed → id landed → npcKind is valid
			cs.NpcKind = npcKind
			// Capture the still-unidentified structured u32s between npcKind and the
			// model path (the config section) verbatim — nothing dropped.
			end := modelOff
			if end <= 0 || end > len(rec) {
				end = len(rec)
			}
			for c.Pos()+4 <= end && len(cs.Fields) < 128 {
				cs.Fields = append(cs.Fields, c.U32())
			}
		}
		out[key] = cs
	}
	return out
}

// getKnowledgeCard extracts N from a "getknowledge(N);" interaction script; 0 when
// the script isn't a knowledge grant.
func getKnowledgeCard(script string) uint32 {
	const pre = "getknowledge("
	if !strings.HasPrefix(script, pre) {
		return 0
	}
	rest := script[len(pre):]
	end := strings.IndexByte(rest, ')')
	if end < 0 {
		return 0
	}
	n, err := strconv.ParseUint(rest[:end], 10, 32)
	if err != nil {
		return 0
	}
	return uint32(n)
}

// modelPathAt returns the record's asset model path — the longest printable-ASCII
// run containing a '/' — and the byte offset it starts at (-1 if none).
func modelPathAt(rec []byte) (string, int) {
	var best []byte
	bestOff := -1
	start := -1
	for i := 0; i <= len(rec); i++ {
		if i < len(rec) && rec[i] >= 0x20 && rec[i] < 0x7f {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			if run := rec[start:i]; len(run) > len(best) && bytes.IndexByte(run, '/') > 0 {
				best, bestOff = run, start
			}
			start = -1
		}
	}
	return string(best), bestOff
}

// ModelPrefix is the lowercased leading segment of a model path (kind fallback).
func ModelPrefix(model string) string {
	if i := strings.IndexByte(model, '/'); i > 0 {
		return strings.ToLower(model[:i])
	}
	return ""
}
