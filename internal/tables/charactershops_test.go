package tables

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestDecodeCharacterItemServices(t *testing.T) {
	t.Parallel()

	var record bytes.Buffer
	_ = binary.Write(&record, binary.LittleEndian, uint16(0x010c))
	_ = binary.Write(&record, binary.LittleEndian, uint16(0x0600))
	writeTestUTF16(&record, "Secret Shop")
	writeTestUTF16(&record, "!checktime(2,22);")
	_ = binary.Write(&record, binary.LittleEndian, uint16(51101))

	rows, err := DecodeCharacterItemServices(testU16OffsetIndex(40068, uint32(record.Len())), record.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	got, ok := rows[40068]
	if !ok {
		t.Fatal("shop row missing")
	}
	if got.SourceName != "Secret Shop" || got.ConditionDSL != "!checktime(2,22);" || got.Unknown0 != 0x010c || got.UnknownKey != 51101 {
		t.Fatalf("unexpected shop: %+v", got)
	}
}

func TestDecodeCharacterItemServicesRejectsUnexpectedTag(t *testing.T) {
	t.Parallel()

	record := make([]byte, 4)
	binary.LittleEndian.PutUint16(record[2:], 0x0601)
	_, err := DecodeCharacterItemServices(testU16OffsetIndex(123, uint32(len(record))), record)
	if err == nil {
		t.Fatal("expected layout error")
	}
}

func TestDecodeCharacterItemServicesOmitsEmptyModule(t *testing.T) {
	t.Parallel()

	var record bytes.Buffer
	_ = binary.Write(&record, binary.LittleEndian, uint16(0x0100))
	_ = binary.Write(&record, binary.LittleEndian, uint16(0x0600))
	writeTestUTF16(&record, "")
	writeTestUTF16(&record, "")
	_ = binary.Write(&record, binary.LittleEndian, uint16(0))

	rows, err := DecodeCharacterItemServices(testU16OffsetIndex(123, uint32(record.Len())), record.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("got %d rows, want none", len(rows))
	}
}

func testU16OffsetIndex(key uint16, size uint32) []byte {
	index := append([]byte("PABR"), make([]byte, 14)...)
	binary.LittleEndian.PutUint32(index[4:], 1)
	binary.LittleEndian.PutUint16(index[8:], key)
	binary.LittleEndian.PutUint32(index[10:], 0)
	binary.LittleEndian.PutUint32(index[14:], size)

	return index
}
