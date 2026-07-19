package tables

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

func TestDecodeCrystalGroupRules(t *testing.T) {
	records := appendU32(nil, 0)
	records = appendU16(records, 101)
	records = appendU16(records, 1)
	records = appendU32(records, 1)
	records = appendU16(records, 103)
	records = appendU16(records, 6)

	rules, err := DecodeCrystalGroupRules(testPABRWithUTF16Strings(2, records, "Ancient Spirit", "Dawn"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 || rules[0].Key != 101 || rules[0].Max != 1 || rules[0].SourceName != "Ancient Spirit" {
		t.Fatalf("first rule = %#v", rules)
	}
	if rules[1].Key != 103 || rules[1].Max != 6 || rules[1].SourceName != "Dawn" {
		t.Fatalf("second rule = %#v", rules[1])
	}
}

func TestDecodeCrystalSpecialSlotRules(t *testing.T) {
	records := []byte{14}
	records = appendU32(records, 1)
	records = appendU16(records, 101)
	records = append(records, 17)
	records = appendU32(records, 2)
	records = appendU16(records, 101)
	records = appendU16(records, 103)

	rules, err := DecodeCrystalSpecialSlotRules(testPABR(2, records))
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 || rules[0].Slot != 14 || len(rules[0].AllowedGroups) != 1 || rules[0].AllowedGroups[0] != 101 {
		t.Fatalf("first rule = %#v", rules)
	}
	if rules[1].Slot != 17 || len(rules[1].AllowedGroups) != 2 || rules[1].AllowedGroups[1] != 103 {
		t.Fatalf("second rule = %#v", rules[1])
	}
}

func testPABRWithUTF16Strings(rows uint32, records []byte, values ...string) []byte {
	data := append([]byte("PABR"), 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(data[4:8], rows)
	data = append(data, records...)
	stringTableOffset := len(data)
	data = appendU32(data, uint32(len(values)))
	data = append(data, 1)
	for _, value := range values {
		units := utf16.Encode([]rune(value))
		data = appendU32(data, uint32(len(units)*2))
		for _, unit := range units {
			data = appendU16(data, unit)
		}
		data = append(data, 1)
	}
	var pointer [8]byte
	binary.LittleEndian.PutUint64(pointer[:], uint64(stringTableOffset))
	return append(data, pointer[:]...)
}
