package tables

import (
	"fmt"
	"math"
	"sort"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

const (
	journalQuestPrefixSize = 128
	familyStatBlockSize    = 49
	questRewardStride      = 178
	questBaseRewardSlots   = 5
)

// AdventureJournalRow is one journal group from journalquest.dbss.
type AdventureJournalRow struct {
	Key               uint32
	SourceName        string
	SourceDescription string
	Books             []AdventureJournalBookRow
}

// AdventureJournalBookRow is one indexed record from journalquest.dbss.
type AdventureJournalBookRow struct {
	Key                      uint32
	SourceJournalName        string
	SourceJournalDescription string
	SourceName               string
	SourceRequirement        string
	Icon                     string
	Texture                  string
	Unknown8                 bool
	Pages                    []JournalPageQuestRow
}

// JournalPageQuestUnknowns preserves the unidentified prefix fields used to
// locate the journal-page quest subtype.
type JournalPageQuestUnknowns struct {
	Unknown4       uint32
	Unknown20      uint32
	Unknown24To127 [104]byte
}

// JournalPageQuestRow is the quest identity and permanent family-stat reward
// needed by the adventure bookshelf.
type JournalPageQuestRow struct {
	Group    uint16
	Index    uint16
	Bonus    *model.FamilyStatBonus
	Unknowns JournalPageQuestUnknowns
}

// FamilyStatQuestRow is one permanent family-stat reward and its quest slot.
type FamilyStatQuestRow struct {
	Group      uint16
	Index      uint16
	RewardSlot uint8
	Bonus      model.FamilyStatBonus
}

type journalBookIndexEntry struct {
	Key    uint32
	Offset uint32
	Size   uint32
}

type journalGroupIndex struct {
	Key     uint32
	Entries []journalBookIndexEntry
}

// DecodeAdventureJournals joins the journal bookshelf, the current quest list,
// and journal-page quest records into ordered books and permanent stat rewards.
func DecodeAdventureJournals(indexData, journalData, questListData, questData []byte) ([]AdventureJournalRow, []FamilyStatQuestRow, error) {
	groups, err := decodeJournalIndex(indexData, len(journalData))
	if err != nil {
		return nil, nil, err
	}
	questIDs, questCount, err := decodeQuestList(questListData)
	if err != nil {
		return nil, nil, err
	}
	if len(questData) < 4 || int(bss.U32(questData, 0)) != questCount {
		return nil, nil, fmt.Errorf("quest: row count does not match allquestlist count %d", questCount)
	}

	rows, wanted, err := decodeJournalBooks(groups, journalData, questIDs)
	if err != nil {
		return nil, nil, err
	}
	pages, err := locateJournalQuestPages(questData, wanted)
	if err != nil {
		return nil, nil, err
	}
	for journalIndex := range rows {
		for bookIndex := range rows[journalIndex].Books {
			book := &rows[journalIndex].Books[bookIndex]
			for pageIndex := range book.Pages {
				id := packQuestID(book.Pages[pageIndex].Group, book.Pages[pageIndex].Index)
				book.Pages[pageIndex] = pages[id]
			}
		}
	}
	bonuses, err := locateFamilyStatQuestRewards(questData, questIDs)
	if err != nil {
		return nil, nil, err
	}
	return rows, bonuses, nil
}

func decodeJournalIndex(data []byte, journalSize int) ([]journalGroupIndex, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("journalquest: truncated index")
	}
	c := bss.NewCursor(data, 0, len(data))
	groupCount := int(c.U32())
	if groupCount <= 0 || groupCount > 256 {
		return nil, fmt.Errorf("journalquest: invalid group count %d", groupCount)
	}
	groups := make([]journalGroupIndex, 0, groupCount)
	seenGroups := make(map[uint32]bool, groupCount)
	for groupIndex := 0; groupIndex < groupCount; groupIndex++ {
		group := journalGroupIndex{Key: c.U32()}
		bookCount := int(c.U32())
		if group.Key == 0 || seenGroups[group.Key] || bookCount <= 0 || bookCount > 256 {
			return nil, fmt.Errorf("journalquest: invalid group %d with %d books", group.Key, bookCount)
		}
		seenGroups[group.Key] = true
		group.Entries = make([]journalBookIndexEntry, 0, bookCount)
		seenBooks := make(map[uint32]bool, bookCount)
		for bookIndex := 0; bookIndex < bookCount; bookIndex++ {
			entry := journalBookIndexEntry{Key: c.U32(), Offset: c.U32(), Size: c.U32()}
			end := uint64(entry.Offset) + uint64(entry.Size)
			if entry.Key == 0 || seenBooks[entry.Key] || entry.Size == 0 || end > uint64(journalSize) {
				return nil, fmt.Errorf("journalquest: group %d has invalid book index entry %+v", group.Key, entry)
			}
			seenBooks[entry.Key] = true
			group.Entries = append(group.Entries, entry)
		}
		groups = append(groups, group)
	}
	if !c.OK() || c.Remaining() != 0 {
		return nil, fmt.Errorf("journalquest: index consumed %d of %d bytes", c.Pos(), len(data))
	}
	return groups, nil
}

func decodeQuestList(data []byte) (map[uint32]bool, int, error) {
	pabr, err := bss.OpenPABR(data)
	if err != nil {
		return nil, 0, fmt.Errorf("allquestlist: %w", err)
	}
	recordSize, fixed := pabr.RecordSize()
	if !fixed || recordSize != 4 {
		return nil, 0, fmt.Errorf("allquestlist: record size %d, want 4", recordSize)
	}
	ids := make(map[uint32]bool, pabr.Rows)
	for row := 0; row < pabr.Rows; row++ {
		id := bss.U32(data, pabr.RecordsStart+row*recordSize)
		if id == 0 || ids[id] {
			return nil, 0, fmt.Errorf("allquestlist: invalid or duplicate quest id %08x", id)
		}
		ids[id] = true
	}
	return ids, pabr.Rows, nil
}

func decodeJournalBooks(groups []journalGroupIndex, data []byte, questIDs map[uint32]bool) ([]AdventureJournalRow, map[uint32]bool, error) {
	if len(data) < 4 || int(bss.U32(data, 0)) != len(groups) {
		return nil, nil, fmt.Errorf("journalquest: data group count does not match index count %d", len(groups))
	}
	if err := validateJournalTiling(groups, data); err != nil {
		return nil, nil, err
	}
	rows := make([]AdventureJournalRow, 0, len(groups))
	wanted := make(map[uint32]bool)
	for _, group := range groups {
		journal := AdventureJournalRow{Key: group.Key, Books: make([]AdventureJournalBookRow, 0, len(group.Entries))}
		for _, entry := range group.Entries {
			book, ids, err := decodeJournalBook(data, entry, group.Key)
			if err != nil {
				return nil, nil, err
			}
			if len(journal.Books) == 0 {
				journal.SourceName = book.SourceJournalName
				journal.SourceDescription = book.SourceJournalDescription
			} else if journal.SourceName != book.SourceJournalName || journal.SourceDescription != book.SourceJournalDescription {
				journal.SourceName = ""
				journal.SourceDescription = ""
			}
			book.Pages = make([]JournalPageQuestRow, 0, len(ids))
			for _, id := range ids {
				if !questIDs[id] {
					return nil, nil, fmt.Errorf("journalquest: group %d book %d references absent quest %08x", group.Key, entry.Key, id)
				}
				if wanted[id] {
					return nil, nil, fmt.Errorf("journalquest: duplicate page quest %08x", id)
				}
				wanted[id] = true
				book.Pages = append(book.Pages, JournalPageQuestRow{Group: uint16(id), Index: uint16(id >> 16)})
			}
			journal.Books = append(journal.Books, book)
		}
		rows = append(rows, journal)
	}
	return rows, wanted, nil
}

func validateJournalTiling(groups []journalGroupIndex, data []byte) error {
	type span struct {
		group uint32
		entry journalBookIndexEntry
	}
	spans := make([]span, 0)
	counts := make(map[uint32]int, len(groups))
	for _, group := range groups {
		counts[group.Key] = len(group.Entries)
		for _, entry := range group.Entries {
			spans = append(spans, span{group: group.Key, entry: entry})
		}
	}
	sort.Slice(spans, func(i, j int) bool {
		return spans[i].entry.Offset < spans[j].entry.Offset
	})
	expected := uint32(4)
	seenGroupHeader := make(map[uint32]bool, len(groups))
	for _, current := range spans {
		if !seenGroupHeader[current.group] {
			if current.entry.Offset != expected+4 || int(bss.U32(data, int(expected))) != counts[current.group] {
				return fmt.Errorf("journalquest: group %d has invalid data count header at %d", current.group, expected)
			}
			seenGroupHeader[current.group] = true
			expected += 4
		}
		if current.entry.Offset != expected {
			return fmt.Errorf("journalquest: book %d in group %d starts at %d, want %d", current.entry.Key, current.group, current.entry.Offset, expected)
		}
		expected += current.entry.Size
	}
	if int(expected) != len(data) || len(seenGroupHeader) != len(groups) {
		return fmt.Errorf("journalquest: indexed records end at %d, data ends at %d", expected, len(data))
	}
	return nil
}

func decodeJournalBook(data []byte, entry journalBookIndexEntry, groupKey uint32) (AdventureJournalBookRow, []uint32, error) {
	record := bss.NewCursor(data, int(entry.Offset), int(entry.Offset+entry.Size))
	journalKey := record.U32()
	bookKey := record.U32()
	unknown8 := record.U8()
	sourceName := record.UTF16()
	sourceDescription := record.UTF16()
	sourceBookName := record.UTF16()
	sourceRequirement := record.UTF16()
	icon := record.UTF8()
	texture := record.UTF8()
	questIDs := record.U32List(1024)
	reservedEnd := record.U32()
	if !record.OK() || record.Remaining() != 0 {
		return AdventureJournalBookRow{}, nil, fmt.Errorf("journalquest: group %d book %d does not consume its %d-byte record", groupKey, entry.Key, entry.Size)
	}
	if journalKey != groupKey || bookKey != entry.Key || unknown8 > 1 || reservedEnd != 0 || len(questIDs) == 0 {
		return AdventureJournalBookRow{}, nil, fmt.Errorf("journalquest: group %d book %d has invalid identity or footer", groupKey, entry.Key)
	}
	return AdventureJournalBookRow{
		Key:                      bookKey,
		SourceJournalName:        sourceName,
		SourceJournalDescription: sourceDescription,
		SourceName:               sourceBookName,
		SourceRequirement:        sourceRequirement,
		Icon:                     icon,
		Texture:                  texture,
		Unknown8:                 unknown8 != 0,
	}, questIDs, nil
}

func locateJournalQuestPages(data []byte, wanted map[uint32]bool) (map[uint32]JournalPageQuestRow, error) {
	matches := make(map[uint32][]int, len(wanted))
	minimum := journalQuestPrefixSize + familyStatBlockSize
	for pos := 4; pos+minimum <= len(data); pos++ {
		id := bss.U32(data, pos)
		if !wanted[id] || !bss.AllZero(data[pos+8:pos+20]) || bss.U32(data, pos+20)&0xff != 7 {
			continue
		}
		matches[id] = append(matches[id], pos)
	}
	pages := make(map[uint32]JournalPageQuestRow, len(wanted))
	for id := range wanted {
		positions := matches[id]
		if len(positions) != 1 {
			return nil, fmt.Errorf("quest: journal page %d-%d has %d structural matches", uint16(id), uint16(id>>16), len(positions))
		}
		page, err := decodeJournalQuestPage(data, positions[0])
		if err != nil {
			return nil, err
		}
		pages[id] = page
	}
	return pages, nil
}

func decodeJournalQuestPage(data []byte, start int) (JournalPageQuestRow, error) {
	c := bss.NewCursor(data, start, start+journalQuestPrefixSize+familyStatBlockSize)
	id := c.U32()
	unknown4 := c.U32()
	if !c.Zero(12) {
		return JournalPageQuestRow{}, fmt.Errorf("quest: journal page %d-%d has nonzero reserved prefix", uint16(id), uint16(id>>16))
	}
	unknown20 := c.U32()
	unknownBytes := c.Bytes(104)
	bonus, err := decodeFamilyStatBonus(c)
	if err != nil {
		return JournalPageQuestRow{}, fmt.Errorf("quest: journal page %d-%d: %w", uint16(id), uint16(id>>16), err)
	}
	if !c.OK() || c.Remaining() != 0 {
		return JournalPageQuestRow{}, fmt.Errorf("quest: journal page %d-%d did not consume reward prefix", uint16(id), uint16(id>>16))
	}
	page := JournalPageQuestRow{Group: uint16(id), Index: uint16(id >> 16), Bonus: bonus}
	page.Unknowns.Unknown4 = unknown4
	page.Unknowns.Unknown20 = unknown20
	copy(page.Unknowns.Unknown24To127[:], unknownBytes)
	return page, nil
}

func decodeFamilyStatBonus(c *bss.Cursor) (*model.FamilyStatBonus, error) {
	statType := model.FamilyStatType(c.U32())
	offence := c.F32()
	defence := c.F32()
	hp := c.F32()
	mp := c.F32()
	stamina := float64(c.I32())
	weight := float64(c.I32()) / 10_000
	inventory := float64(c.U8())
	accuracy := c.F32()
	evasion := c.F32()
	enhancementChance := float64(c.I32())
	valksLimit := float64(c.I32())
	stackLimit := float64(c.I32())
	values := map[model.FamilyStatType]float64{
		model.FamilyStatTypeOffence:           offence,
		model.FamilyStatTypeDefence:           defence,
		model.FamilyStatTypeHP:                hp,
		model.FamilyStatTypeMP:                mp,
		model.FamilyStatTypeStamina:           stamina,
		model.FamilyStatTypeWeight:            weight,
		model.FamilyStatTypeInventory:         inventory,
		model.FamilyStatTypeAccuracy:          accuracy,
		model.FamilyStatTypeEvasion:           evasion,
		model.FamilyStatTypeEnhancementChance: enhancementChance,
		model.FamilyStatTypeValksLimit:        valksLimit,
		model.FamilyStatTypeStackLimit:        stackLimit,
	}
	if !c.OK() {
		return nil, fmt.Errorf("truncated family-stat block")
	}
	if statType == model.FamilyStatTypeNone {
		for _, value := range values {
			if value != 0 {
				return nil, fmt.Errorf("none reward has a nonzero family-stat value")
			}
		}
		return nil, nil
	}
	value, ok := values[statType]
	if !ok {
		return nil, fmt.Errorf("unknown family-stat type %d", statType)
	}
	for candidate, otherValue := range values {
		if candidate != statType && otherValue != 0 {
			return nil, fmt.Errorf("family-stat type %d also populates type %d", statType, candidate)
		}
	}
	if value == 0 {
		return nil, fmt.Errorf("family-stat type %d has a zero value", statType)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 1 || value > 10_000 || value != math.Trunc(value) {
		return nil, fmt.Errorf("family-stat type %d has invalid value %g", statType, value)
	}
	bonus := &model.FamilyStatBonus{Type: statType, Value: value}
	if statType == model.FamilyStatTypeWeight {
		bonus.Unit = "LT"
	}
	return bonus, nil
}

func locateFamilyStatQuestRewards(data []byte, questIDs map[uint32]bool) ([]FamilyStatQuestRow, error) {
	minimum := journalQuestPrefixSize + (questBaseRewardSlots-1)*questRewardStride + familyStatBlockSize
	rows := make([]FamilyStatQuestRow, 0)
	seen := make(map[uint32]bool)
	for pos := 4; pos+minimum <= len(data); pos++ {
		id := bss.U32(data, pos)
		if !questIDs[id] || !bss.AllZero(data[pos+8:pos+20]) || bss.U32(data, pos+20)&0xff != 7 {
			continue
		}
		if seen[id] {
			return nil, fmt.Errorf("quest %d-%d has multiple family-stat records", uint16(id), uint16(id>>16))
		}
		seen[id] = true
		for slot := 0; slot < questBaseRewardSlots; slot++ {
			rewardPos := pos + journalQuestPrefixSize + slot*questRewardStride
			statType := model.FamilyStatType(bss.U32(data, rewardPos))
			if statType > model.FamilyStatTypeStackLimit {
				continue
			}
			c := bss.NewCursor(data, rewardPos, rewardPos+familyStatBlockSize)
			bonus, err := decodeFamilyStatBonus(c)
			if err != nil || bonus == nil {
				continue
			}
			rows = append(rows, FamilyStatQuestRow{
				Group:      uint16(id),
				Index:      uint16(id >> 16),
				RewardSlot: uint8(slot),
				Bonus:      *bonus,
			})
		}
	}
	return rows, nil
}

func packQuestID(group, index uint16) uint32 {
	return uint32(index)<<16 | uint32(group)
}
