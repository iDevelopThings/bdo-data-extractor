package tables

import (
	"bytes"
	"encoding/binary"
	"testing"
	"unicode/utf16"

	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

func TestDecodeItemRowVariablePostIconStrings(t *testing.T) {
	const id = 1234
	record := make([]byte, 208)
	binary.LittleEndian.PutUint32(record, id)
	record[4] = eItemTypeEquip
	record[7] = 1
	record[12] = 2
	record[14] = 0
	record[15] = 0
	for i := 16; i < 62; i++ {
		record[i] = slotNone
	}
	record[161] = 255
	record[162] = 255
	record[169] = 1
	record[203] = 1

	record = appendUTF8(record, "New_Icon/test.dds")
	post := []byte{1, 0, 9, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 2, 0, 2, 1, 0}
	record = append(record, post...)
	record = appendUTF16(record, "first")
	record = appendUTF16(record, "second")
	record = appendUTF16(record, "third")
	record = binary.LittleEndian.AppendUint64(record, 500)

	record = append(record, 0)
	record = binary.LittleEndian.AppendUint32(record, 0)
	record = binary.LittleEndian.AppendUint32(record, 0)
	record = append(record, 0x77, 0x77, 0x77)
	for range 43 {
		record = append(record, 0x77)
	}
	record = append(record, 0, 0, 0, 0, 0)
	for range 6 {
		record = binary.LittleEndian.AppendUint32(record, 1_000_000)
	}
	record = append(record, 0xaa, 0xbb)
	record = binary.LittleEndian.AppendUint32(record, id)
	record = binary.LittleEndian.AppendUint16(record, noJewelGroup)
	record = binary.LittleEndian.AppendUint16(record, 0)

	stat, err := decodeItemRow(record, id)
	if err != nil {
		t.Fatal(err)
	}
	if stat.MarketRegisterLimit != 500 {
		t.Fatalf("market limit = %d, want 500", stat.MarketRegisterLimit)
	}
	if got := stat.U.UnknownPostIconStrings; got != [3]string{"first", "second", "third"} {
		t.Fatalf("post-icon strings = %q", got)
	}
	if got := stat.U.UnknownPostIconTail; len(got) != 2 || got[0] != 0xaa || got[1] != 0xbb {
		t.Fatalf("opaque tail = %x, want aabb", got)
	}
}

func TestOwnItemTailsDetachesSource(t *testing.T) {
	source := []byte{1, 2, 3, 4, 5}
	stats := map[uint32]ItemStat{
		1: {U: model.ItemUnknowns{UnknownPostIconTail: source[:2]}},
		2: {U: model.ItemUnknowns{UnknownPostIconTail: source[2:]}},
	}

	ownItemTails(stats)
	clear(source)

	if got := stats[1].U.UnknownPostIconTail; !bytes.Equal(got, []byte{1, 2}) || cap(got) != len(got) {
		t.Fatalf("first owned tail = %v len/cap %d/%d", got, len(got), cap(got))
	}
	if got := stats[2].U.UnknownPostIconTail; !bytes.Equal(got, []byte{3, 4, 5}) || cap(got) != len(got) {
		t.Fatalf("second owned tail = %v len/cap %d/%d", got, len(got), cap(got))
	}
}

func appendUTF8(dst []byte, value string) []byte {
	dst = binary.LittleEndian.AppendUint64(dst, uint64(len(value)))
	return append(dst, value...)
}

func appendUTF16(dst []byte, value string) []byte {
	units := utf16.Encode([]rune(value))
	dst = binary.LittleEndian.AppendUint64(dst, uint64(len(units)))
	for _, unit := range units {
		dst = binary.LittleEndian.AppendUint16(dst, unit)
	}
	return dst
}
