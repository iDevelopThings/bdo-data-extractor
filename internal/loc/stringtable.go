package loc

import (
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
)

// SymbolicStringTable is the localization table joined by stringtable.bss.
const SymbolicStringTable uint32 = 37

// Table is one localization table indexed by id and packed field selector.
type Table map[uint32]map[uint32]string

// Lookup returns the localized text for an id and field selector.
func (t Table) Lookup(id, field uint32) (string, bool) {
	fields, ok := t[id]
	if !ok {
		return "", false
	}
	text, ok := fields[field]
	return text, ok
}

// LuaString is one symbolic PAGetString key and its resolved text.
type LuaString struct {
	ID             uint32 `json:"id"`
	Field          uint32 `json:"field"`
	Source         string `json:"source"`
	Text           string `json:"text"`
	SourceFallback bool   `json:"sourceFallback,omitempty"`
}

// LuaStringSheet is one Defines.StringSheet_* namespace.
type LuaStringSheet struct {
	Hash    uint32               `json:"hash"`
	Field   uint32               `json:"field"`
	Strings map[string]LuaString `json:"strings"`
}

// LuaStringCatalog maps client string-sheet names and symbolic keys to text.
type LuaStringCatalog struct {
	Sheets map[string]LuaStringSheet `json:"sheets"`
}

// Lookup resolves a symbolic key within a client string sheet.
func (c *LuaStringCatalog) Lookup(sheet, key string) (LuaString, bool) {
	if c == nil {
		return LuaString{}, false
	}
	s, ok := c.Sheets[sheet]
	if !ok {
		return LuaString{}, false
	}
	value, ok := s.Strings[key]
	return value, ok
}

var luaStringSheetFields = map[string]uint32{
	"CUTSCENE":    0,
	"GAME":        1,
	"RESOURCE":    2,
	"ACTIONCHART": 3,
	"TOOL":        4,
	"WEB":         5,
	"SymbolNo":    6,
	"IMAGESLIDE":  7,
}

// DecodeLuaStrings decodes gamecommondata/binary/stringtable.bss.
func DecodeLuaStrings(data []byte) (*LuaStringCatalog, error) {
	pabr, err := bss.OpenPABR(data)
	if err != nil {
		return nil, fmt.Errorf("stringtable: %w", err)
	}
	strings := bss.ReadUTF16StringTable(data, pabr.StringTablePos)
	if len(strings) == 0 {
		return nil, fmt.Errorf("stringtable: empty or malformed string table")
	}

	catalog := &LuaStringCatalog{Sheets: make(map[string]LuaStringSheet, pabr.Rows)}
	pos := pabr.RecordsStart
	for row := range pabr.Rows {
		c := bss.NewCursor(data, pos, pabr.StringTablePos)
		hash := c.U32()
		nameIndex := int(c.U32())
		count := int(c.U32())
		if nameIndex < 0 || nameIndex >= len(strings) {
			return nil, fmt.Errorf("stringtable row %d: sheet string index %d out of range", row, nameIndex)
		}
		if count < 0 || count > c.Remaining()/16 {
			return nil, fmt.Errorf("stringtable row %d: entry count %d exceeds record area", row, count)
		}
		name := strings[nameIndex]
		field, ok := luaStringSheetFields[name]
		if !ok {
			return nil, fmt.Errorf("stringtable row %d: unknown sheet %q", row, name)
		}
		if _, exists := catalog.Sheets[name]; exists {
			return nil, fmt.Errorf("stringtable row %d: duplicate sheet %q", row, name)
		}
		sheet := LuaStringSheet{Hash: hash, Field: field, Strings: make(map[string]LuaString, count)}
		for entry := range count {
			id := c.U32()
			keyIndex := int(c.U32())
			sourceIndex := int(c.U32())
			reserved := c.U32()
			if keyIndex < 0 || keyIndex >= len(strings) || sourceIndex < 0 || sourceIndex >= len(strings) {
				return nil, fmt.Errorf("stringtable row %d entry %d: string index out of range", row, entry)
			}
			if reserved != 0 {
				return nil, fmt.Errorf("stringtable row %d entry %d: reserved value %d", row, entry, reserved)
			}
			key := strings[keyIndex]
			if _, exists := sheet.Strings[key]; exists {
				return nil, fmt.Errorf("stringtable row %d entry %d: duplicate key %q", row, entry, key)
			}
			source := strings[sourceIndex]
			sheet.Strings[key] = LuaString{
				ID:             id,
				Field:          field,
				Source:         source,
				Text:           source,
				SourceFallback: true,
			}
		}
		if !c.OK() {
			return nil, fmt.Errorf("stringtable row %d: truncated at %d", row, c.Pos())
		}
		pos = c.Pos()
		catalog.Sheets[name] = sheet
	}
	if pos != pabr.StringTablePos {
		return nil, fmt.Errorf("stringtable: records end at %d, string table starts at %d", pos, pabr.StringTablePos)
	}
	return catalog, nil
}

// Resolve applies localization table 37 to every symbolic string. The sheet's
// base field is preferred; packed alternate field 0x10000 is used when needed.
func (c *LuaStringCatalog) Resolve(table Table) {
	if c == nil {
		return
	}
	for name, sheet := range c.Sheets {
		for key, value := range sheet.Strings {
			value.Field = sheet.Field
			value.Text = value.Source
			value.SourceFallback = true
			if text, ok := table.Lookup(value.ID, sheet.Field); ok {
				value.Text = text
				value.SourceFallback = false
			} else {
				alternate := sheet.Field | 0x10000
				if text, ok := table.Lookup(value.ID, alternate); ok {
					value.Field = alternate
					value.Text = text
					value.SourceFallback = false
				}
			}
			sheet.Strings[key] = value
		}
		c.Sheets[name] = sheet
	}
}
