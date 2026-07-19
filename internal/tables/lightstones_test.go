package tables

import (
	"bytes"
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

func TestDecodeLightstoneCombinations(t *testing.T) {
	t.Parallel()

	var records bytes.Buffer
	writeRow := func(key, skill uint32, required []uint32, description uint32) {
		writeBinary(t, &records, key)
		writeBinary(t, &records, skill)
		writeBinary(t, &records, uint16(0))
		for _, itemID := range required {
			writeBinary(t, &records, itemID)
		}
		writeBinary(t, &records, description)
	}
	writeRow(2, 317222, []uint32{758003, 762004, 762005}, 0)
	writeRow(4, 317224, []uint32{758003, 758004, 762004, 762005}, 1)
	writeBinary(t, &records, uint32(2))
	for _, alias := range [][2]uint32{{758003, 758003}, {758203, 758003}} {
		writeBinary(t, &records, alias[0])
		writeBinary(t, &records, alias[1])
	}

	var data bytes.Buffer
	data.WriteString("PABR")
	writeBinary(t, &data, uint32(2))
	data.Write(records.Bytes())
	stringTablePos := uint64(data.Len())
	writeUTF16StringTable(t, &data, []string{"three", "four"})
	writeBinary(t, &data, stringTablePos)

	rows, aliases, err := DecodeLightstoneCombinations(data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || len(rows[0].RequiredItems) != 3 || len(rows[1].RequiredItems) != 4 {
		t.Fatalf("rows = %#v", rows)
	}
	if rows[0].DescriptionKR != "three" || rows[1].DescriptionKR != "four" {
		t.Fatalf("descriptions = %q, %q", rows[0].DescriptionKR, rows[1].DescriptionKR)
	}
	if got := rows[0].SkillIndexKey(); got != 0xd7260001 {
		t.Fatalf("skill index key = %#x", got)
	}
	if len(aliases) != 2 || aliases[1].Item != 758203 || aliases[1].CountsAs != 758003 {
		t.Fatalf("aliases = %#v", aliases)
	}
}

func writeUTF16StringTable(t *testing.T, dst *bytes.Buffer, values []string) {
	t.Helper()
	writeBinary(t, dst, uint32(len(values)))
	dst.WriteByte(1)
	for _, value := range values {
		units := utf16.Encode([]rune(value))
		writeBinary(t, dst, uint32(len(units)*2))
		for _, unit := range units {
			writeBinary(t, dst, unit)
		}
		dst.WriteByte(1)
	}
}

func writeBinary[T ~uint16 | ~uint32 | ~uint64](t *testing.T, dst *bytes.Buffer, value T) {
	t.Helper()
	if err := binary.Write(dst, binary.LittleEndian, value); err != nil {
		t.Fatal(err)
	}
}
