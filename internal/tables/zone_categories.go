package tables

import (
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
)

// DecodeMainCategoryIcons decodes dropuimaincategoryinfo (caller must ICE-decrypt
// it first — it ships double-encrypted). Layout: 12 fixed 10-byte rows
// [u32 key][u16 order][u16 iconIdx][u16 0] then an ASCII string table of tab-icon
// ids. Returns key -> icon id.
func DecodeMainCategoryIcons(data []byte) map[uint32]string {
	return decodeCategoryIcons(data, 10, 6, true)
}

// DecodeSubCategoryIcons decodes dropuisubcategoryinfo: 8 fixed 12-byte rows
// [u32 key][u32 nameIdx][u32 iconIdx] then a string table (UTF-16 names + ASCII
// filter-button icons). Returns key -> icon id.
func DecodeSubCategoryIcons(data []byte) map[uint32]string {
	return decodeCategoryIcons(data, 12, 8, false)
}

// decodeCategoryIcons walks fixed-width rows of recSize bytes (key @0, icon
// string index at iconOff as u16 or u32) and resolves each to its icon string.
func decodeCategoryIcons(data []byte, recSize, iconOff int, iconU16 bool) map[uint32]string {
	h, err := bss.OpenPABR(data)
	if err != nil {
		return nil
	}
	strs := bss.ReadStringTable(data, h.StringTablePos)
	out := make(map[uint32]string, h.Rows)
	for i := 0; i < h.Rows; i++ {
		o := h.RecordsStart + i*recSize
		if o+recSize > len(data) {
			break
		}
		idx := int(bss.U32(data, o+iconOff))
		if iconU16 {
			idx = int(bss.U16(data, o+iconOff))
		}
		if idx >= 0 && idx < len(strs) {
			out[bss.U32(data, o)] = trimIconState(strs[idx])
		}
	}
	return out
}

// trimIconState drops a trailing UI-state suffix so every category resolves to
// its base sprite (sub icons are stored with _Over; main icons have none). The
// resting/Normal sprite is what the in-game bar shows.
func trimIconState(s string) string {
	for _, suf := range []string{"_Over", "_Normal", "_Click"} {
		if strings.HasSuffix(s, suf) {
			return s[:len(s)-len(suf)]
		}
	}
	return s
}
