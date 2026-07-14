package tables

import (
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
)

func TestDecodePlantNodeProducts(t *testing.T) {
	plantData := binary.LittleEndian.AppendUint32(nil, 3)
	plantRows := [][]byte{
		plantProductRow(10, 100, []byte{0, 1}),
		plantProductRow(11, 1<<16|100, []byte{2}),
		plantProductRow(12, 101, []byte{0}),
	}
	plantIndex := binary.LittleEndian.AppendUint32(nil, uint32(len(plantRows)))
	for i, row := range plantRows {
		offset := len(plantData)
		plantData = append(plantData, row...)
		plantIndex = binary.LittleEndian.AppendUint32(plantIndex, uint32(10+i))
		plantIndex = binary.LittleEndian.AppendUint32(plantIndex, uint32(offset))
		plantIndex = binary.LittleEndian.AppendUint32(plantIndex, uint32(len(row)))
	}

	exchangeData := plantExchangeTable(
		[2]uint32{100, 500},
		[2]uint32{101, 501},
	)
	subgroupData := binary.LittleEndian.AppendUint32(nil, 1)
	subgroupOffset := len(subgroupData)
	subgroup := itemSubgroupRow(500, 1000, 2000)
	subgroupData = append(subgroupData, subgroup...)
	subgroupIndex := append([]byte("PABR"), make([]byte, 4)...)
	binary.LittleEndian.PutUint32(subgroupIndex[4:], 1)
	subgroupIndex = binary.LittleEndian.AppendUint16(subgroupIndex, 500)
	subgroupIndex = binary.LittleEndian.AppendUint32(subgroupIndex, uint32(subgroupOffset))
	subgroupIndex = binary.LittleEndian.AppendUint32(subgroupIndex, uint32(len(subgroup)))

	got, err := DecodePlantNodeProducts(
		plantData, plantIndex, exchangeData, subgroupData, subgroupIndex,
	)
	if err != nil {
		t.Fatalf("DecodePlantNodeProducts: %v", err)
	}
	for _, node := range []uint32{10, 11} {
		if want := []uint32{1000, 2000}; !reflect.DeepEqual(got.ByNode[node], want) {
			t.Errorf("node %d products = %v, want %v", node, got.ByNode[node], want)
		}
	}
	if want := []uint32{12}; !reflect.DeepEqual(got.UnresolvedNodes, want) {
		t.Errorf("unresolved nodes = %v, want %v", got.UnresolvedNodes, want)
	}
}

func TestDecodeItemSubgroupRejectsTrailingData(t *testing.T) {
	record := append(itemSubgroupRow(500, 1000), 0)
	_, err := decodeItemSubgroup(record, bss.IndexEntry{Key: 500, Size: uint32(len(record))})
	if err == nil {
		t.Fatal("decodeItemSubgroup accepted trailing data")
	}
}

func plantProductRow(node, production uint32, species []byte) []byte {
	row := binary.LittleEndian.AppendUint32(nil, node)
	row = append(row, make([]byte, 19)...)
	row = binary.LittleEndian.AppendUint32(row, production)
	row = binary.LittleEndian.AppendUint32(row, uint32(len(species)))
	return append(row, species...)
}

func plantExchangeTable(entries ...[2]uint32) []byte {
	data := append([]byte("PABR"), make([]byte, 4)...)
	binary.LittleEndian.PutUint32(data[4:], uint32(len(entries)))
	for _, entry := range entries {
		row := binary.LittleEndian.AppendUint16(nil, uint16(entry[0]))
		row = binary.LittleEndian.AppendUint16(row, uint16(entry[0]))
		row = binary.LittleEndian.AppendUint16(row, 0)
		row = binary.LittleEndian.AppendUint32(row, entry[1])
		row = append(row, make([]byte, 80)...)
		row = binary.LittleEndian.AppendUint32(row, 0)
		data = append(data, row...)
	}
	stringTableOffset := len(data)
	data = append(data, make([]byte, 8)...)
	binary.LittleEndian.PutUint64(data[len(data)-8:], uint64(stringTableOffset))
	return data
}

func itemSubgroupRow(key uint32, items ...uint32) []byte {
	row := binary.LittleEndian.AppendUint32(nil, key)
	row = append(row, make([]byte, 10)...)
	row = binary.LittleEndian.AppendUint32(row, uint32(len(items)))
	for _, item := range items {
		row = binary.LittleEndian.AppendUint32(row, item)
		row = append(row, make([]byte, 131)...)
	}
	return row
}
