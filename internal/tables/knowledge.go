package tables

import (
	"bytes"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
	"github.com/idevelopthings/bdo-data-extractor/src/models"
	"github.com/idevelopthings/bdo-data-extractor/src/utils"
)

// DecodeKnowledgeThemes decodes mentaltheme.dbss via its offset companion: a
// u32-count header then 10-byte index records [u16 key, u32 offset, u32 size].
// Each data record is [u16 key, i64 nameLen, nameLen*2 UTF-16 name, u16 parent];
// the embedded name is the source language, so the English name comes from loc.
func DecodeKnowledgeThemes(offsetRaw, dataRaw []byte, names map[uint32]string) ([]model.KnowledgeTheme, error) {
	out := make([]model.KnowledgeTheme, 0)
	for rec, err := range bss.IndexedRecordsU16("mentaltheme", offsetRaw, dataRaw) {
		if err != nil {
			return nil, err
		}
		out = append(out, model.KnowledgeTheme{
			BaseFor: models.NewBaseFor[model.KnowledgeTheme](rec.Entry.Key, "theme"),
			Key:     rec.Entry.Key,
			Name:    names[rec.Entry.Key],
			Parent:  model.ThemeRef(themeParent(dataRaw, int(rec.Entry.Offset))),
		})
	}

	return out, nil
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

// DecodeKnowledgeEntries decodes mentalcard.dbss via its PABR offset companion as a
// sequential field stream (bss.Cursor). English name/desc come from loc; the image is
// reached by its .dds anchor, since the embedded strings are variable-length. The
// subject kind is the theme category, not a header field — see model.KnowledgeEntry.
// Record layout: see FORMATS.md, "mentalcard.dbss".
func DecodeKnowledgeEntries(offsetRaw, dataRaw []byte, names, descs, acquire map[uint32]string) ([]model.KnowledgeEntry, error) {
	out := make([]model.KnowledgeEntry, 0)
	for e, err := range bss.IndexedRecords(offsetRaw, dataRaw) {
		if err != nil {
			return nil, err
		}
		rec := e.Data
		if len(rec) < 40 {
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
				BaseFor:     models.NewBaseFor[model.KnowledgeEntry](e.Entry.Key, "entry"),
				Key:         e.Entry.Key,
				Theme:       model.ThemeRef(theme),
				Name:        names[e.Entry.Key],
				Description: descs[e.Entry.Key],
				Acquisition: acquire[e.Entry.Key],
				Image:       imageName(rec),
				MinFavor:    round2(minFavor),
				MaxFavor:    round2(maxFavor),
				Interest:    round2(interest),
				Unknown20:   dev(flags, 4),
			},
		)
	}

	return out, nil
}

// imageName returns the card's embedded image as a lowercase, PAZ-relative asset
// path (e.g. "ui_artwork/ic_09996.webp") — matching what `knowledge-icons` writes,
// so the viewer can load knowledge_icons/<image> verbatim. The embedded path is
// mixed-case .dds relative to ui_texture/.
func imageName(rec []byte) string {
	p := ddsPath(rec)
	if p == "" {
		return ""
	}

	return utils.IconFileName(strings.ToLower(p))
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
