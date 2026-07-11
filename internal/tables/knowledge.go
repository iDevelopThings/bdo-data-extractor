package tables

import (
	"bytes"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
	"github.com/idevelopthings/bdo-data-extractor/src/models"
)

// DecodeKnowledgeThemes decodes mentaltheme.dbss via its offset companion: a
// u32-count header then 10-byte index records [u16 key, u32 offset, u32 size].
// Each data record is [u16 key, i64 nameLen, nameLen*2 UTF-16 name, u16 parent];
// the embedded name is the source language, so the English name comes from loc.
func DecodeKnowledgeThemes(offsetRaw, dataRaw []byte, names map[uint32]string) []model.KnowledgeTheme {
	const rec = 10
	if len(offsetRaw) < 4 {
		return nil
	}
	count := int(bss.U32(offsetRaw, 0))
	if count <= 0 || 4+count*rec > len(offsetRaw) {
		return nil
	}

	out := make([]model.KnowledgeTheme, 0, count)
	for i := 0; i < count; i++ {
		o := 4 + i*rec
		key := uint32(bss.U16(offsetRaw, o))
		doff := int(bss.U32(offsetRaw, o+2))
		out = append(out, model.KnowledgeTheme{
			BaseFor: models.NewBaseFor[model.KnowledgeTheme](key, "theme"),
			Key:     key,
			Name:    names[key],
			Parent:  model.ThemeRef(themeParent(dataRaw, doff)),
		})
	}

	return out
}

// themeParent reads the parent theme key that follows the name in a mentaltheme
// record: [u16 key][i64 nameLen][nameLen*2 UTF-16][u16 parent].
func themeParent(data []byte, off int) uint32 {
	if off < 0 || off+10 > len(data) {
		return 0
	}
	nameLen := int(bss.U64(data, off+2))
	p := off + 10 + nameLen*2
	if nameLen < 0 || p+2 > len(data) {
		return 0
	}

	return uint32(bss.U16(data, p))
}

// DecodeKnowledgeEntries decodes mentalcard.dbss via its PABR offset companion,
// reading each record as a straight sequential field stream (a bss.Cursor, no
// offset-jumping) — the fixed 40-byte header is fully mapped:
//
//	@0  u32 key · @4 u32 theme · @8/@12/@16 f32 minFavor/maxFavor/interest ·
//	@20 u32 flags · @24/@28/@32 u32 (unidentified) · @36 u32 reserved (=0) ·
//	@40… embedded source-language name/desc + the .dds image path
//
// English name/desc come from loc; the image is reached by its .dds anchor (the
// embedded strings are variable-length, like the item icon). The subject KIND is
// the theme category, not a header field — see model.KnowledgeEntry.
func DecodeKnowledgeEntries(offsetRaw, dataRaw []byte, names, descs, acquire map[uint32]string) []model.KnowledgeEntry {
	idx, err := bss.ParseOffsetIndex(offsetRaw, len(dataRaw))
	if err != nil {
		return nil
	}

	out := make([]model.KnowledgeEntry, 0, len(idx))
	for _, e := range idx {
		rec, ok := e.Slice(dataRaw)
		if !ok || len(rec) < 40 {
			continue
		}
		c := bss.NewCursor(rec, 0, len(rec))
		c.U32()               // @0  key (== e.Key)
		theme := c.U32()      // @4
		minFavor := c.F32()   // @8
		maxFavor := c.F32()   // @12
		interest := c.F32()   // @16
		flags := int(c.U32()) // @20  flags bitfield (obtain/display; not the kind)
		c.U32()               // @24  ┐ packed sub-structure (u32 reads come back as
		c.U32()               // @28  │ N<<8 — misaligned; byte widths not yet cracked),
		c.U32()               // @32  │ read through so the cursor lands on the name;
		c.U32()               // @36  ┘ populated only on ~2.5k non-default cards.

		out = append(
			out, model.KnowledgeEntry{
				BaseFor:     models.NewBaseFor[model.KnowledgeEntry](e.Key, "entry"),
				Key:         e.Key,
				Theme:       model.ThemeRef(theme),
				Name:        names[e.Key],
				Description: descs[e.Key],
				Acquisition: acquire[e.Key],
				Image:       imageName(rec),
				MinFavor:    round2(minFavor),
				MaxFavor:    round2(maxFavor),
				Interest:    round2(interest),
				Unknown20:   dev(flags, 4),
			},
		)
	}

	return out
}

// imageName returns the card's embedded image as a lowercase, PAZ-relative .png
// path (e.g. "ui_artwork/ic_09996.png") — matching what `knowledge-icons` writes,
// so the viewer can load knowledge_icons/<image> verbatim. The embedded path is
// mixed-case .dds relative to ui_texture/.
func imageName(rec []byte) string {
	p := ddsPath(rec)
	if p == "" {
		return ""
	}

	return strings.ToLower(strings.TrimSuffix(p, ".dds")) + ".png"
}

// ddsPath returns the .dds asset path embedded as ASCII in a record, or "".
func ddsPath(rec []byte) string {
	i := bytes.Index(rec, []byte(".dds"))
	if i < 0 {
		return ""
	}
	start := i
	for start > 0 && rec[start-1] >= 0x20 && rec[start-1] < 0x7f {
		start--
	}

	return string(rec[start : i+4])
}
