package tables

import (
	"bytes"
	"encoding/binary"
	"math"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

func TestDecodeAdventureJournals(t *testing.T) {
	t.Parallel()
	questIDs := []uint32{packQuestID(748, 1), packQuestID(748, 2), packQuestID(748, 3), packQuestID(748, 4)}
	journalData, indexData := journalFixture(t, questIDs)
	questList := questListFixture(questIDs)
	questData := questFixture(
		journalQuestFixture(questIDs[0], model.FamilyStatTypeHP, 30),
		journalQuestFixture(questIDs[1], model.FamilyStatTypeWeight, 2),
		journalQuestFixture(questIDs[2], model.FamilyStatTypeAccuracy, 4),
		journalQuestFixture(questIDs[3], model.FamilyStatTypeNone, 0),
	)

	rows, bonuses, err := DecodeAdventureJournals(indexData, journalData, questList, questData)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0].Books) != 1 || len(rows[0].Books[0].Pages) != 4 {
		t.Fatalf("decoded shape = %d journals, %d books, %d pages", len(rows), len(rows[0].Books), len(rows[0].Books[0].Pages))
	}
	if len(bonuses) != 3 {
		t.Fatalf("family bonuses = %d, want 3", len(bonuses))
	}
	book := rows[0].Books[0]
	if rows[0].SourceName != "Journal" || book.SourceName != "Book" || book.SourceRequirement != "Requirement" {
		t.Fatalf("decoded source text = journal %q, book %q, requirement %q", rows[0].SourceName, book.SourceName, book.SourceRequirement)
	}
	want := []*model.FamilyStatBonus{
		{Type: model.FamilyStatTypeHP, Value: 30},
		{Type: model.FamilyStatTypeWeight, Value: 2, Unit: "LT"},
		{Type: model.FamilyStatTypeAccuracy, Value: 4},
		nil,
	}
	for i, page := range book.Pages {
		if page.Bonus == nil || want[i] == nil {
			if page.Bonus != want[i] {
				t.Fatalf("page %d bonus = %+v, want %+v", i, page.Bonus, want[i])
			}
			continue
		}
		if *page.Bonus != *want[i] {
			t.Fatalf("page %d bonus = %+v, want %+v", i, page.Bonus, want[i])
		}
	}
}

func TestDecodeAdventureJournalsRequiresOneStructuralQuestMatch(t *testing.T) {
	t.Parallel()
	id := packQuestID(748, 1)
	journalData, indexData := journalFixture(t, []uint32{id})
	questList := questListFixture([]uint32{id})
	record := journalQuestFixture(id, model.FamilyStatTypeHP, 30)
	copy(record[8:20], bytes.Repeat([]byte{1}, 12))
	_, _, err := DecodeAdventureJournals(indexData, journalData, questList, questFixture(record))
	if err == nil || !strings.Contains(err.Error(), "0 structural matches") {
		t.Fatalf("error = %v, want missing structural match", err)
	}
}

func journalFixture(t *testing.T, questIDs []uint32) ([]byte, []byte) {
	t.Helper()
	record := new(bytes.Buffer)
	writeLE(record, uint32(7))
	writeLE(record, uint32(3))
	record.WriteByte(1)
	writeUTF16(record, "Journal")
	writeUTF16(record, "Description")
	writeUTF16(record, "Book")
	writeUTF16(record, "Requirement")
	writeUTF8(record, "Icon")
	writeUTF8(record, "Texture")
	writeLE(record, uint32(len(questIDs)))
	for _, id := range questIDs {
		writeLE(record, id)
	}
	writeLE(record, uint32(0))

	data := new(bytes.Buffer)
	writeLE(data, uint32(1))
	writeLE(data, uint32(1))
	data.Write(record.Bytes())

	index := new(bytes.Buffer)
	writeLE(index, uint32(1))
	writeLE(index, uint32(7))
	writeLE(index, uint32(1))
	writeLE(index, uint32(3))
	writeLE(index, uint32(8))
	writeLE(index, uint32(record.Len()))
	return data.Bytes(), index.Bytes()
}

func questListFixture(ids []uint32) []byte {
	data := new(bytes.Buffer)
	data.WriteString("PABR")
	writeLE(data, uint32(len(ids)))
	for _, id := range ids {
		writeLE(data, id)
	}
	stringTablePos := uint64(data.Len())
	writeLE(data, stringTablePos)
	return data.Bytes()
}

func questFixture(records ...[]byte) []byte {
	data := new(bytes.Buffer)
	writeLE(data, uint32(len(records)))
	for _, record := range records {
		data.Write(record)
	}
	return data.Bytes()
}

func journalQuestFixture(id uint32, statType model.FamilyStatType, value float64) []byte {
	record := make([]byte, journalQuestPrefixSize+(questBaseRewardSlots-1)*questRewardStride+familyStatBlockSize)
	binary.LittleEndian.PutUint32(record, id)
	binary.LittleEndian.PutUint32(record[4:], 1)
	binary.LittleEndian.PutUint32(record[20:], 7)
	binary.LittleEndian.PutUint32(record[128:], uint32(statType))
	switch statType {
	case model.FamilyStatTypeOffence:
		binary.LittleEndian.PutUint32(record[132:], math.Float32bits(float32(value)))
	case model.FamilyStatTypeDefence:
		binary.LittleEndian.PutUint32(record[136:], math.Float32bits(float32(value)))
	case model.FamilyStatTypeHP:
		binary.LittleEndian.PutUint32(record[140:], math.Float32bits(float32(value)))
	case model.FamilyStatTypeMP:
		binary.LittleEndian.PutUint32(record[144:], math.Float32bits(float32(value)))
	case model.FamilyStatTypeStamina:
		binary.LittleEndian.PutUint32(record[148:], uint32(value))
	case model.FamilyStatTypeWeight:
		binary.LittleEndian.PutUint32(record[152:], uint32(value*10_000))
	case model.FamilyStatTypeInventory:
		record[156] = byte(value)
	case model.FamilyStatTypeAccuracy:
		binary.LittleEndian.PutUint32(record[157:], math.Float32bits(float32(value)))
	case model.FamilyStatTypeEvasion:
		binary.LittleEndian.PutUint32(record[161:], math.Float32bits(float32(value)))
	case model.FamilyStatTypeEnhancementChance:
		binary.LittleEndian.PutUint32(record[165:], uint32(value))
	case model.FamilyStatTypeValksLimit:
		binary.LittleEndian.PutUint32(record[169:], uint32(value))
	case model.FamilyStatTypeStackLimit:
		binary.LittleEndian.PutUint32(record[173:], uint32(value))
	}
	return record
}

func writeUTF16(buf *bytes.Buffer, value string) {
	encoded := utf16.Encode([]rune(value))
	writeLE(buf, int64(len(encoded)))
	for _, code := range encoded {
		writeLE(buf, code)
	}
}

func writeUTF8(buf *bytes.Buffer, value string) {
	writeLE(buf, int64(len(value)))
	buf.WriteString(value)
}

func writeLE[T ~uint16 | ~uint32 | ~uint64 | ~int64](buf *bytes.Buffer, value T) {
	if err := binary.Write(buf, binary.LittleEndian, value); err != nil {
		panic(err)
	}
}
