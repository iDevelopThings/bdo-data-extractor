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
//
// Loc strings may contain Pearl Abyss UI markup (<PA…> color/font tags). LoadGame
// leaves that markup intact so consumers can style it.
package loc

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/klauspost/compress/zlib"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
)

// loc key0 values are stable semantic categories (not shifting indices), so
// selecting tables by these constants is safe across patches.
const (
	itemTable             = 0
	titleTable            = 1 // character title names
	buffNameTable         = 5
	knowledgeThemeTable   = 9  // knowledge category/theme names
	skillTable            = 10 // player and non-player skill names/descriptions
	entityNameTable       = 6  // general entity-name table: classes, creatures, NPCs, resources (id-ranged)
	knowledgeCardTable    = 34 // knowledge card (entry) names
	mainCatTable          = 12 // Monster Zone Info main-category (region) names
	topographyTable       = 17 // topography / place names
	nodeNameTable         = 29 // worldmap node names, keyed by exploration node key
	questTable            = 18 // quest names, keyed (group=id, index=key1)
	marketCatTable        = 44
	itemSetTable          = 52  // skillpiece set-effect text, keyed by skillNo and apply index
	adventureJournalTable = 63  // adventure journals, keyed by journal group and book key
	lightstoneSetTable    = 113 // lightstone combination text, keyed by combination key
	houseTable            = 123 // house function / workshop names, keyed by house type (eHouseIconType)
	jewelGroupTable       = 121 // crystal transfusion groups: id=group, key1=max count, text=name
	subCatTable           = 115 // Monster Zone Info sub-category (content filter) names
	zoneNameTable         = 116 // Monster Zone Info zone names (sparse ids, in record order)
	tagTable              = 117 // Monster Zone Info tag labels (keys 1..44)
	fieldName             = 0x00000000
	fieldDesc             = 0x01000000
	fieldCol2             = 0x02000000 // items: use/open confirmation text; knowledge cards: acquisition hint
	fieldCol3             = 0x03000000 // items: NPC exchange info
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

// Text is the common name + description pair used by most loc tables.
type Text struct {
	Name        string
	Description string
}

// ItemText is one item's loc columns from table 0 (name/desc/use/exchange).
type ItemText struct {
	Name        string
	Description string
	Use         string
	Exchange    string
}

// KnowledgeCardText is one knowledge-card entry from table 34.
type KnowledgeCardText struct {
	Name        string
	Description string
	Acquire     string
}

// EntityText is one general entity row from table 6 (name + NPC-style title).
type EntityText struct {
	Name  string
	Title string // e.g. "<Farm Vendor>"
}

// TerritoryText is one table-12 row: nation grouping plus finer territory name.
type TerritoryText struct {
	Nation string // field 0 (was MainCatNames)
	Name   string // desc field (was TerritoryNames)
}

// AdventureJournalText is one localized adventure-journal book and its parent
// journal presentation text.
type AdventureJournalText struct {
	JournalName        string
	JournalDescription string
	Name               string
	Requirement        string
}

// GameStrings is everything the build needs from the .loc, decoded in one pass:
// item text (table 0), market categories (table 44), buff effect text (table 5),
// and the various inline-name tables later stages resolve against.
type GameStrings struct {
	Items             map[uint32]ItemText                        // item id -> name/desc/use/exchange (table 0)
	MarketCats        map[uint32]MarketCat                       // central-market category id -> name + subs (table 44)
	BuffNames         map[uint32]string                          // buff id -> full decoded text, may be multi-line + <PA> tags (table 5)
	ItemSetTexts      Table                                      // skillpiece text (table 52), indexed by skillNo and packed field
	AdventureJournals map[uint32]map[uint32]AdventureJournalText // journal group -> book key -> text (table 63)
	LightstoneSets    map[uint32]Text                            // lightstone combination key -> name/description (table 113)
	Skills            map[uint32]Text                            // skill number -> localized name/description (table 10)
	// Monster Zone Info linkage (inline names):
	Titles      map[uint32]Text                 // title id -> name + requirement/description (table 1)
	Entities    map[uint32]EntityText           // general entity table: classes/creatures/NPCs/resources (table 6); Title holds "<Farm Vendor>"-style tags
	Territories map[uint32]TerritoryText        // territory id -> nation (field 0) + territory name (desc) (table 12)
	Topography  map[uint32]string               // place/topography id -> name (table 17)
	NodeNames   map[uint32]string               // worldmap node key -> name (table 29)
	Quests      map[uint32]map[uint32]QuestText // quest group -> index -> texts (table 18)
	SubCatNames map[uint32]string               // sub-category (content filter) id -> name (table 115)
	Tags        map[uint32]Text                 // tag key -> label + description (table 117)
	ZoneNames   []string                        // table 116, non-null in id order (== zone record order)
	ThemeNames  map[uint32]string               // knowledge theme key -> name (table 9)
	Cards       map[uint32]KnowledgeCardText    // knowledge card key -> name/desc/acquire (table 34)
	HouseNames  map[uint32]string               // house type -> workshop name (table 123)
	JewelGroups map[uint32]JewelGroup           // crystal group id -> name + max count (table 121)
}

// decodeLocString UTF-16-decodes one loc field, drops the "<null>" sentinel, and
// trims surrounding whitespace. Pearl Abyss <PA…> markup inside the string is kept.
func decodeLocString(raw []byte) string {
	if t := bss.DecodeUTF16(raw); t != "<null>" {
		return strings.TrimSpace(t)
	}
	return ""
}

// setNameDesc assigns fieldName / fieldDesc into m[id]; other key1 values are ignored.
func setNameDesc(m map[uint32]Text, id, key1 uint32, s string) {
	if s == "" {
		return
	}
	t := m[id]
	switch key1 {
	case fieldName:
		t.Name = s
	case fieldDesc:
		t.Description = s
	default:
		return
	}
	m[id] = t
}

// setItemField assigns item table 0 columns 0..3 into Items[id].
func setItemField(items map[uint32]ItemText, id, key1 uint32, s string) {
	if s == "" {
		return
	}
	it := items[id]
	switch key1 {
	case fieldName:
		it.Name = s
	case fieldDesc:
		it.Description = s
	case fieldCol2:
		it.Use = s
	case fieldCol3:
		it.Exchange = s
	default:
		return
	}
	items[id] = it
}

// setKnowledgeCardField assigns card name / description / acquire columns.
func setKnowledgeCardField(cards map[uint32]KnowledgeCardText, id, key1 uint32, s string) {
	if s == "" {
		return
	}
	c := cards[id]
	switch key1 {
	case fieldName:
		c.Name = s
	case fieldDesc:
		c.Description = s
	case fieldCol2:
		c.Acquire = s
	default:
		return
	}
	cards[id] = c
}

// setEntityField assigns entity name (field 0) or title (field 1).
func setEntityField(entities map[uint32]EntityText, id, key1 uint32, s string) {
	if s == "" {
		return
	}
	e := entities[id]
	switch key1 {
	case fieldName:
		e.Name = s
	case fieldDesc:
		e.Title = s
	default:
		return
	}
	entities[id] = e
}

// setTerritoryField assigns nation (field 0) or territory name (desc field).
func setTerritoryField(territories map[uint32]TerritoryText, id, key1 uint32, s string) {
	if s == "" {
		return
	}
	t := territories[id]
	switch key1 {
	case fieldName:
		t.Nation = s
	case fieldDesc:
		t.Name = s
	default:
		return
	}
	territories[id] = t
}

// setLightstoneText stores table 113's single-field blob as Description and
// derives Name from the first line (optional [brackets]), leaving <PA> tags intact.
func setLightstoneText(sets map[uint32]Text, id uint32, blob string) {
	if blob == "" {
		return
	}
	name, _, _ := strings.Cut(blob, "\n")
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(strings.TrimPrefix(name, "["), "]")
	sets[id] = Text{Name: name, Description: blob}
}

// setBuffName stores table 5 text (full multi-line blob, first write wins).
func setBuffName(names map[uint32]string, id, key1 uint32, s string) {
	if key1 != 0 || s == "" {
		return
	}
	if _, ok := names[id]; ok {
		return
	}
	names[id] = s
}

// LoadGame decodes the tables the build needs in a single pass, decoding only
// those tables rather than the whole 1.4M-string file.
func LoadGame(gameDir, lang string) (*GameStrings, error) {
	body, err := readBody(gameDir, lang)
	if err != nil {
		return nil, err
	}
	gs := &GameStrings{
		Items:             map[uint32]ItemText{},
		MarketCats:        map[uint32]MarketCat{},
		BuffNames:         map[uint32]string{},
		ItemSetTexts:      make(Table),
		AdventureJournals: map[uint32]map[uint32]AdventureJournalText{},
		LightstoneSets:    map[uint32]Text{},
		Skills:            map[uint32]Text{},
		Titles:            map[uint32]Text{},
		Entities:          map[uint32]EntityText{},
		Territories:       map[uint32]TerritoryText{},
		Topography:        map[uint32]string{},
		NodeNames:         map[uint32]string{},
		Quests:            map[uint32]map[uint32]QuestText{},
		SubCatNames:       map[uint32]string{},
		Tags:              map[uint32]Text{},
		ThemeNames:        map[uint32]string{},
		Cards:             map[uint32]KnowledgeCardText{},
		HouseNames:        map[uint32]string{},
		JewelGroups:       map[uint32]JewelGroup{},
	}
	zone116 := map[uint32]string{}
	forEachRecord(body, func(key0, id, key1 uint32, raw []byte) {
		switch key0 {
		case itemTable:
			setItemField(gs.Items, id, key1, decodeLocString(raw))
		case marketCatTable:
			t := decodeLocString(raw)
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
			setBuffName(gs.BuffNames, id, key1, decodeLocString(raw))
		case itemSetTable:
			if t := decodeLocString(raw); t != "" {
				if gs.ItemSetTexts[id] == nil {
					gs.ItemSetTexts[id] = make(map[uint32]string)
				}
				gs.ItemSetTexts[id][key1] = t
			}
		case adventureJournalTable:
			setAdventureJournalText(gs.AdventureJournals, id, key1, decodeLocString(raw))
		case lightstoneSetTable:
			if key1 == fieldName {
				setLightstoneText(gs.LightstoneSets, id, decodeLocString(raw))
			}
		case skillTable:
			setNameDesc(gs.Skills, id, key1, decodeLocString(raw))
		case titleTable:
			setNameDesc(gs.Titles, id, key1, decodeLocString(raw))
		case entityNameTable:
			setEntityField(gs.Entities, id, key1, decodeLocString(raw))
		case mainCatTable:
			setTerritoryField(gs.Territories, id, key1, decodeLocString(raw))
		case houseTable:
			if key1 == fieldName {
				if t := decodeLocString(raw); t != "" {
					gs.HouseNames[id] = t
				}
			}
		case jewelGroupTable: // key1 IS the max transfusable count, not a field id
			if t := decodeLocString(raw); t != "" {
				gs.JewelGroups[id] = JewelGroup{Name: t, Max: int(key1)}
			}
		case subCatTable:
			if key1 == fieldName {
				if t := decodeLocString(raw); t != "" {
					gs.SubCatNames[id] = t
				}
			}
		case topographyTable:
			if key1 == fieldName {
				if t := decodeLocString(raw); t != "" {
					gs.Topography[id] = t
				}
			}
		case nodeNameTable:
			if key1 == fieldName {
				if t := decodeLocString(raw); t != "" {
					gs.NodeNames[id] = t
				}
			}
		case questTable: // id = quest group, key1 = plane<<24 | quest index
			plane := key1 >> 24
			if plane > 3 {
				return // planes 4-9 are accept/complete NPC dialogue
			}
			t := decodeLocString(raw)
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
				q.Desc = t
			case 2:
				q.Giver = t
			case 3:
				q.Objective = t
			}
			gs.Quests[id][idx] = q
		case tagTable:
			setNameDesc(gs.Tags, id, key1, decodeLocString(raw))
		case zoneNameTable:
			if key1 == fieldName {
				if t := decodeLocString(raw); t != "" {
					zone116[id] = t
				}
			}
		case knowledgeThemeTable:
			if key1 == fieldName {
				if t := decodeLocString(raw); t != "" {
					gs.ThemeNames[id] = t
				}
			}
		case knowledgeCardTable:
			setKnowledgeCardField(gs.Cards, id, key1, decodeLocString(raw))
		}
	})
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

func setAdventureJournalText(texts map[uint32]map[uint32]AdventureJournalText, group, field uint32, text string) {
	if text == "" || field>>24 > 3 {
		return
	}
	bookKey := field & 0xFFFFFF
	if texts[group] == nil {
		texts[group] = make(map[uint32]AdventureJournalText)
	}
	journal := texts[group][bookKey]
	switch field >> 24 {
	case 0:
		journal.JournalName = text
	case 1:
		journal.JournalDescription = text
	case 2:
		journal.Requirement = text
	case 3:
		journal.Name = text
	}
	texts[group][bookKey] = journal
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
