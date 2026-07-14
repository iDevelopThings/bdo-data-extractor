package tables

import (
	"encoding/binary"
	"reflect"
	"testing"
)

func TestDecodeNodeManagerOwners(t *testing.T) {
	records := map[uint16][]byte{
		100: managerRecord([]uint32{20, 10, 30}),
		200: managerRecord([]uint32{40}),
	}
	index, data := managerFixture(records)

	got, err := DecodeNodeManagerOwners(index, data, map[uint32][]uint32{
		100: {10, 20, 30},
		200: {40},
	})
	if err != nil {
		t.Fatalf("DecodeNodeManagerOwners: %v", err)
	}
	want := map[uint32]uint32{100: 20, 200: 40}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("owners = %v, want %v", got, want)
	}
}

func TestDecodeNodeManagerOwnersRejectsAmbiguousFamily(t *testing.T) {
	record := append(managerRecord([]uint32{10, 20}), managerRecord([]uint32{20, 10})...)
	index, data := managerFixture(map[uint16][]byte{100: record})

	_, err := DecodeNodeManagerOwners(index, data, map[uint32][]uint32{100: {10, 20}})
	if err == nil {
		t.Fatal("expected ambiguous-family error")
	}
}

func managerRecord(nodes []uint32) []byte {
	record := []byte{0xAA, 0xBB, 0xCC}
	record = binary.LittleEndian.AppendUint32(record, uint32(len(nodes)))
	for _, node := range nodes {
		record = binary.LittleEndian.AppendUint32(record, node)
	}
	return append(record, 0xDD, 0xEE)
}

func managerFixture(records map[uint16][]byte) ([]byte, []byte) {
	index := append([]byte("PABR"), make([]byte, 4)...)
	binary.LittleEndian.PutUint32(index[4:], uint32(len(records)))
	data := []byte{0, 0, 0, 0}
	for key, record := range records {
		offset := len(data)
		data = append(data, record...)
		index = binary.LittleEndian.AppendUint16(index, key)
		index = binary.LittleEndian.AppendUint32(index, uint32(offset))
		index = binary.LittleEndian.AppendUint32(index, uint32(len(record)))
	}
	return index, data
}
