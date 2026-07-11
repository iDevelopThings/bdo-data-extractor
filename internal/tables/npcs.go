package tables

import (
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
	"github.com/idevelopthings/bdo-data-extractor/src/models"
)

// DecodeNPCs reads npcsimply.bss. It is a PABR table laid out as
// [PABR][u32 rowCount][rowCount × 33-byte records][string table], where the
// string table is appended at the pointer stored in the last 8 bytes. Each record
// holds the NPC id at u16@0 and two string references at u32@20 / u32@24 (the
// index is the value >> 8) into the (UTF-8) string table: name and title.
func DecodeNPCs(data []byte) ([]model.NPC, error) {
	h, err := bss.OpenPABR(data)
	if err != nil {
		return nil, fmt.Errorf("npcsimply: %w", err)
	}
	strs := bss.ReadStringTable(data, h.StringTablePos)
	str := func(i int) string {
		if i >= 0 && i < len(strs) {
			return strs[i]
		}
		return ""
	}
	recSize := (h.StringTablePos - h.RecordsStart) / h.Rows
	if recSize < 28 {
		return nil, fmt.Errorf("npcsimply: unexpected record size %d", recSize)
	}
	out := make([]model.NPC, 0, h.Rows)
	for i := 0; i < h.Rows; i++ {
		o := h.RecordsStart + i*recSize
		rec := data[o : o+recSize]
		name := str(int(bss.U32(rec, 20) >> 8))
		if name == "" {
			continue
		}
		id := uint32(bss.U16(rec, 0))
		out = append(
			out, model.NPC{
				BaseFor: models.NewBaseFor[model.NPC](id),
				ID:      id,
				Name:    name,
				Title:   str(int(bss.U32(rec, 24) >> 8)),
			},
		)
	}
	return out, nil
}
