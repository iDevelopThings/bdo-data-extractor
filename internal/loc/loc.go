// Package loc parses BDO's ads/languagedata_<lang>.loc localization files.
//
// Format: u32 decompressed_size, then a zlib stream of records:
//
//	u32 text_len   (UTF-16 code units)
//	u32 key0       (string table; 0 = items, 5 = buffs, 44 = market categories)
//	u32 id
//	u32 key1       (packed field selector, see below)
//	text_len*2 bytes UTF-16LE
//	u32 0          terminator (always 0)
//
// Records tile the decompressed body exactly and (key0, id, key1) is unique
// across the file (verified over all 1.38M records).
//
// key1 is a packed column selector, not a flat enum:
//
//	byte 3 (>>24)     field/column index: 0 = name, 1 = description; some
//	                  tables carry more columns (items: 2 = use/open
//	                  confirmation text, 3 = NPC exchange info; knowledge
//	                  cards: 2 = acquisition hint; quests use planes 0-9)
//	byte 2 (>>16)     sub-plane in a few tables (market-category alt labels)
//	bytes 0-1         per-id index (quest index, market sub-category id)
//
// These files live in the game install dir (NOT the PAZ) and are read-only.
package loc

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/klauspost/compress/zlib"
)

// loc key0 values are stable semantic categories (not shifting indices), so
// selecting tables by these constants is safe across patches.
const (
	itemTable           = 0
	titleTable          = 1 // character title names
	buffNameTable       = 5
	knowledgeThemeTable = 9  // knowledge category/theme names
	entityNameTable     = 6  // general entity-name table: classes, creatures, NPCs, resources (id-ranged)
	knowledgeCardTable  = 34 // knowledge card (entry) names
	mainCatTable        = 12 // Monster Zone Info main-category (region) names
	topographyTable     = 17 // topography / place names
	nodeNameTable       = 29 // worldmap node names, keyed by exploration node key
	questTable          = 18 // quest names, keyed (group=id, index=key1)
	marketCatTable      = 44
	itemSetTable        = 52  // skillpiece set-effect text, keyed by skillNo and apply index
	houseTable          = 123 // house function / workshop names, keyed by house type (eHouseIconType)
	jewelGroupTable     = 121 // crystal transfusion groups: id=group, key1=max count, text=name
	subCatTable         = 115 // Monster Zone Info sub-category (content filter) names
	zoneNameTable       = 116 // Monster Zone Info zone names (sparse ids, in record order)
	tagTable            = 117 // Monster Zone Info tag labels (keys 1..44)
	fieldName           = 0x00000000
	fieldDesc           = 0x01000000
	fieldCol2           = 0x02000000 // items: use/open confirmation text; knowledge cards: acquisition hint
	fieldCol3           = 0x03000000 // items: NPC exchange info
)

// readBody returns the decompressed record stream of a .loc file.
func readBody(gameDir, lang string) ([]byte, error) {
	path := filepath.Join(gameDir, "ads", "languagedata_"+lang+".loc")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) < 4 {
		return nil, fmt.Errorf("loc %s: too short (%d bytes)", path, len(raw))
	}
	zr, err := zlib.NewReader(bytes.NewReader(raw[4:]))
	if err != nil {
		return nil, err
	}
	defer func() {
		// ReadFull reports stream errors; Close only releases decoder state.
		_ = zr.Close()
	}()

	declared := int(bss.U32(raw, 0))
	body := make([]byte, declared)
	if _, err := io.ReadFull(zr, body); err != nil {
		return nil, fmt.Errorf("loc %s: read decompressed body: %w", path, err)
	}
	var extra [1]byte
	if _, err := io.ReadFull(zr, extra[:]); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("output exceeds declared size")
		}
		return nil, fmt.Errorf("loc %s: validate decompressed size: %w", path, err)
	}
	return body, nil
}

// forEachRecord walks the decompressed record stream, calling fn with each
// record's raw (still-encoded) UTF-16 text bytes. The caller decodes only the
// records it wants, so callers that need a few tables don't pay to decode all.
func forEachRecord(body []byte, fn func(key0, id, key1 uint32, raw []byte)) {
	for off := 0; off+16 <= len(body); {
		tlen := bss.U32(body, off)
		key0 := bss.U32(body, off+4)
		id := bss.U32(body, off+8)
		key1 := bss.U32(body, off+12)
		off += 16
		end := off + int(tlen)*2
		if end+4 > len(body) {
			break
		}
		fn(key0, id, key1, body[off:end])
		off = end + 4
	}
}

// MarketCat is a central-market category from loc table 44: its display name plus
// its sub-category names (keyed by sub id, matching item byte @201).
type MarketCat struct {
	Name string
	Subs map[uint32]string
}

// QuestText is one quest's text set from loc table 18, planes 0-3 of key1's
// field byte. Planes 4-9 (accept/complete NPC dialogue lines) are not consumed
// but are available via LoadAll / the loc dump.
type QuestText struct {
	Name      string
	Desc      string
	Giver     string
	Objective string
}

// JewelGroup is a crystal transfusion group from loc table 121, keyed by the
// group number in an item's record footer. key1 of the loc record is the max
// transfusable count per group (1000 = no limit).
type JewelGroup struct {
	Name string
	Max  int
}

// GameStrings is everything the build needs from the .loc, decoded in one pass:
// item names/descriptions (table 0), market categories (table 44), and buff
// effect names (table 5).
type GameStrings struct {
	Names         map[uint32]string
	Descs         map[uint32]string
	UseTexts      map[uint32]string // item id -> use/open confirmation text, lists box contents (table 0, column 2)
	ExchangeTexts map[uint32]string // item id -> NPC exchange offers (table 0, column 3)
	MarketCats    map[uint32]MarketCat
	BuffNames     map[uint32]string
	ItemSetTexts  Table // skillpiece text (table 52), indexed by skillNo and packed field
	// Monster Zone Info linkage (inline names):
	Titles         map[uint32]string               // title id -> name (table 1)
	TitleDescs     map[uint32]string               // title id -> requirement/description (table 1 desc field)
	EntityNames    map[uint32]string               // general entity-name table: classes/creatures/NPCs/resources (table 6)
	EntityTitles   map[uint32]string               // similar to above, the "<Farm Vendor>" type tags/titles for npcs
	MainCatNames   map[uint32]string               // main-category (region) id -> nation name (table 12, field 0)
	TerritoryNames map[uint32]string               // territory id -> territory name (table 12, desc field)
	Topography     map[uint32]string               // place/topography id -> name (table 17)
	NodeNames      map[uint32]string               // worldmap node key -> name (table 29)
	Quests         map[uint32]map[uint32]QuestText // quest group -> index -> texts (table 18)
	SubCatNames    map[uint32]string               // sub-category (content filter) id -> name (table 115)
	Tags           map[uint32]string               // tag key -> label (table 117)
	TagDescs       map[uint32]string               // tag key -> description (table 117 desc field)
	ZoneNames      []string                        // table 116, non-null in id order (== zone record order)
	ThemeNames     map[uint32]string               // knowledge theme key -> name (table 9)
	CardNames      map[uint32]string               // knowledge card key -> name (table 34)
	CardDescs      map[uint32]string               // knowledge card key -> description (table 34 desc field)
	CardAcquire    map[uint32]string               // knowledge card key -> acquisition hint (table 34, column 2)
	HouseNames     map[uint32]string               // house type -> workshop name (table 123)
	JewelGroups    map[uint32]JewelGroup           // crystal group id -> name + max count (table 121)
}

var paTag = regexp.MustCompile(`<PA[^>]*>`)

// LoadGame decodes the item, market-category, and buff-name tables in a single
// pass, decoding only those tables rather than the whole 1.4M-string file.
func LoadGame(gameDir, lang string) (*GameStrings, error) {
	body, err := readBody(gameDir, lang)
	if err != nil {
		return nil, err
	}
	gs := &GameStrings{
		Names:          map[uint32]string{},
		Descs:          map[uint32]string{},
		UseTexts:       map[uint32]string{},
		ExchangeTexts:  map[uint32]string{},
		MarketCats:     map[uint32]MarketCat{},
		BuffNames:      map[uint32]string{},
		ItemSetTexts:   make(Table),
		Titles:         map[uint32]string{},
		TitleDescs:     map[uint32]string{},
		EntityNames:    map[uint32]string{},
		EntityTitles:   map[uint32]string{},
		MainCatNames:   map[uint32]string{},
		TerritoryNames: map[uint32]string{},
		Topography:     map[uint32]string{},
		NodeNames:      map[uint32]string{},
		Quests:         map[uint32]map[uint32]QuestText{},
		SubCatNames:    map[uint32]string{},
		Tags:           map[uint32]string{},
		TagDescs:       map[uint32]string{},
		ThemeNames:     map[uint32]string{},
		CardNames:      map[uint32]string{},
		CardDescs:      map[uint32]string{},
		CardAcquire:    map[uint32]string{},
		HouseNames:     map[uint32]string{},
		JewelGroups:    map[uint32]JewelGroup{},
	}
	zone116 := map[uint32]string{} // collected, then flattened in id order
	text := func(raw []byte) string {
		if t := bss.DecodeUTF16(raw); t != "<null>" {
			return t
		}
		return ""
	}
	forEachRecord(body, func(key0, id, key1 uint32, raw []byte) {
		switch key0 {
		case itemTable:
			t := text(raw)
			if t == "" {
				return
			}
			switch key1 {
			case fieldName:
				gs.Names[id] = t
			case fieldDesc:
				gs.Descs[id] = t
			case fieldCol2:
				gs.UseTexts[id] = t
			case fieldCol3:
				gs.ExchangeTexts[id] = t
			}
		case marketCatTable:
			t := text(raw)
			if t == "" {
				return
			}
			c := gs.MarketCats[id]
			if c.Subs == nil {
				c.Subs = map[uint32]string{}
			}
			switch {
			case key1 == 0:
				c.Name = t
			case key1 < 0x10000:
				c.Subs[key1] = t
			}
			gs.MarketCats[id] = c
		case buffNameTable:
			if key1 != 0 {
				return
			}
			name := paTag.ReplaceAllString(text(raw), "")
			if i := strings.IndexByte(name, '\n'); i >= 0 {
				name = name[:i] // some debuffs carry a multi-line description
			}
			if name = strings.TrimSpace(name); name != "" {
				if _, ok := gs.BuffNames[id]; !ok {
					gs.BuffNames[id] = name
				}
			}
		case itemSetTable:
			if t := text(raw); t != "" {
				if gs.ItemSetTexts[id] == nil {
					gs.ItemSetTexts[id] = make(map[uint32]string)
				}
				gs.ItemSetTexts[id][key1] = t
			}
		case titleTable:
			if t := text(raw); t != "" {
				switch key1 {
				case fieldName:
					gs.Titles[id] = t
				case fieldDesc:
					gs.TitleDescs[id] = strings.TrimSpace(paTag.ReplaceAllString(t, ""))
				}
			}
		case entityNameTable:
			if key1 == fieldName {
				if t := strings.TrimSpace(paTag.ReplaceAllString(text(raw), "")); t != "" {
					gs.EntityNames[id] = t
				}
			}
			if key1 == fieldDesc {
				if t := strings.TrimSpace(paTag.ReplaceAllString(text(raw), "")); t != "" {
					gs.EntityTitles[id] = t
				}
			}
		case mainCatTable:
			// field 0 = nation grouping ("Republic of Calpheon"); the desc field
			// (0x01000000) holds the finer territory name ("Balenos").
			if t := text(raw); t != "" {
				switch key1 {
				case fieldName:
					gs.MainCatNames[id] = t
				case fieldDesc:
					gs.TerritoryNames[id] = t
				}
			}
		case houseTable:
			if key1 == fieldName {
				if t := strings.TrimSpace(text(raw)); t != "" {
					gs.HouseNames[id] = t
				}
			}
		case jewelGroupTable: // key1 IS the max transfusable count, not a field id
			if t := strings.TrimSpace(text(raw)); t != "" {
				gs.JewelGroups[id] = JewelGroup{Name: t, Max: int(key1)}
			}
		case subCatTable:
			if key1 == fieldName {
				if t := text(raw); t != "" {
					gs.SubCatNames[id] = t
				}
			}
		case topographyTable:
			if key1 == fieldName {
				if t := text(raw); t != "" {
					gs.Topography[id] = t
				}
			}
		case nodeNameTable:
			if key1 == fieldName {
				if t := text(raw); t != "" {
					gs.NodeNames[id] = t
				}
			}
		case questTable: // id = quest group, key1 = plane<<24 | quest index
			plane := key1 >> 24
			if plane > 3 {
				return // planes 4-9 are accept/complete NPC dialogue
			}
			t := text(raw)
			if t == "" {
				return
			}
			idx := key1 & 0xFFFFFF
			if gs.Quests[id] == nil {
				gs.Quests[id] = map[uint32]QuestText{}
			}
			q := gs.Quests[id][idx]
			switch plane {
			case 0:
				q.Name = t
			case 1:
				q.Desc = strings.TrimSpace(paTag.ReplaceAllString(t, ""))
			case 2:
				q.Giver = t
			case 3:
				q.Objective = t
			}
			gs.Quests[id][idx] = q
		case tagTable:
			if t := text(raw); t != "" {
				switch key1 {
				case fieldName:
					gs.Tags[id] = t
				case fieldDesc:
					gs.TagDescs[id] = t
				}
			}
		case zoneNameTable:
			if key1 == fieldName {
				if t := text(raw); t != "" {
					zone116[id] = t
				}
			}
		case knowledgeThemeTable:
			if key1 == fieldName {
				if t := strings.TrimSpace(paTag.ReplaceAllString(text(raw), "")); t != "" {
					gs.ThemeNames[id] = t
				}
			}
		case knowledgeCardTable:
			if t := strings.TrimSpace(paTag.ReplaceAllString(text(raw), "")); t != "" {
				switch key1 {
				case fieldName:
					gs.CardNames[id] = t
				case fieldDesc:
					gs.CardDescs[id] = t
				case fieldCol2:
					gs.CardAcquire[id] = t
				}
			}
		}
	})
	// flatten table 116 to record order: non-null entries sorted by id.
	ids := make([]int, 0, len(zone116))
	for id := range zone116 {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	for _, id := range ids {
		gs.ZoneNames = append(gs.ZoneNames, zone116[uint32(id)])
	}
	return gs, nil
}

// Record is one localized string with its raw keys (for the `loc` dump command).
type Record struct {
	Key0 uint32 `json:"key0"`
	ID   uint32 `json:"id"`
	Key1 uint32 `json:"key1"`
	Text string `json:"text"`
}

// LoadAll decodes every record of a .loc file (all tables/fields), unfiltered.
func LoadAll(gameDir, lang string) ([]Record, error) {
	body, err := readBody(gameDir, lang)
	if err != nil {
		return nil, err
	}
	var recs []Record
	forEachRecord(body, func(key0, id, key1 uint32, raw []byte) {
		recs = append(recs, Record{Key0: key0, ID: id, Key1: key1, Text: bss.DecodeUTF16(raw)})
	})
	return recs, nil
}

// LoadTable decodes one localization table, indexed by id and packed field.
func LoadTable(gameDir, lang string, key uint32) (Table, error) {
	body, err := readBody(gameDir, lang)
	if err != nil {
		return nil, err
	}
	table := make(Table)
	forEachRecord(body, func(key0, id, field uint32, raw []byte) {
		if key0 != key {
			return
		}
		if table[id] == nil {
			table[id] = make(map[uint32]string)
		}
		table[id][field] = bss.DecodeUTF16(raw)
	})
	return table, nil
}
