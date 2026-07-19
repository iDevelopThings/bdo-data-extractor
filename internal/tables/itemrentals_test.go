package tables

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
)

func TestDecodeItemRentals(t *testing.T) {
	t.Parallel()

	var record bytes.Buffer
	writeTestUTF16(&record, "!getitemcount(23004,0)>0;getlevel()>44;checkClass(0);")
	writeTestUTF16(&record, "[Rental] Kaia Longsword")
	_ = binary.Write(&record, binary.LittleEndian, uint16(7))
	_ = binary.Write(&record, binary.LittleEndian, uint16(0))
	writeTestUTF16(&record, "A contribution-point weapon.")
	writeTestUTF16(&record, "buyItemByPoint(23004,0,1,5,50)")

	data := record.Bytes()
	offset := testOffsetIndex(42152|uint32(1)<<16, uint32(len(data)))
	rows, err := DecodeItemRentals(offset, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	got := rows[0]
	if got.CharacterKey != 42152 || got.DialogIndex != 1 || got.ItemKey != 23004 || got.Count != 1 || got.PointType != 5 || got.PointCost != 50 {
		t.Fatalf("unexpected rental: %+v", got)
	}
	if got.Unknown0 != 7 || got.ItemSubKey != 0 {
		t.Fatalf("unexpected preserved fields: %+v", got)
	}
}

func TestDecodeItemRentalsRejectsMalformedAction(t *testing.T) {
	t.Parallel()

	var record bytes.Buffer
	writeTestUTF16(&record, "getlevel()>44;")
	writeTestUTF16(&record, "Rental")
	_ = binary.Write(&record, binary.LittleEndian, uint32(0))
	writeTestUTF16(&record, "Description")
	writeTestUTF16(&record, "buyItemByPoint(23004,0,1)")

	if _, err := decodeItemRentalRecord(record.Bytes()); err == nil {
		t.Fatal("direct decoder accepted malformed rental action")
	}
	_, err := DecodeItemRentals(testOffsetIndex(42152, uint32(record.Len())), record.Bytes())
	if err == nil {
		t.Fatal("expected malformed rental action error")
	}
}

func TestDecodeItemRentalsPrefersEmptyConditionOverEmbeddedNullCandidate(t *testing.T) {
	t.Parallel()

	var record bytes.Buffer
	writeTestUTF16(&record, "\x00\x00")
	writeTestUTF16(&record, "Rental")
	_ = binary.Write(&record, binary.LittleEndian, uint32(0))
	writeTestUTF16(&record, "Description")
	writeTestUTF16(&record, "buyItemByPoint(23004,0,1,5,50)")

	rows, err := DecodeItemRentals(testOffsetIndex(42152, uint32(record.Len())), record.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ConditionDSL != "" {
		t.Fatalf("rows = %+v, want one unconditional rental", rows)
	}
}

func writeTestUTF16(buf *bytes.Buffer, value string) {
	encoded := bss.EncodeUTF16(value)
	_ = binary.Write(buf, binary.LittleEndian, int64(len(encoded)/2))
	_, _ = buf.Write(encoded)
}

func testOffsetIndex(key, size uint32) []byte {
	data := append([]byte("PABR"), make([]byte, 16)...)
	binary.LittleEndian.PutUint32(data[4:], 1)
	binary.LittleEndian.PutUint32(data[8:], key)
	binary.LittleEndian.PutUint32(data[12:], 0)
	binary.LittleEndian.PutUint32(data[16:], size)

	return data
}
