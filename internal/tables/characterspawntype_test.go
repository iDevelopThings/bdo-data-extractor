package tables

import (
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

func TestDecodeCharacterSpawnTypes(t *testing.T) {
	index, data := characterSpawnTypeFixture([]spawnTypeTestRow{
		{key: 40029, types: []int{4, 10, 12}},
		{key: 50010, types: []int{2, 4}},
	})

	got, err := DecodeCharacterSpawnTypes(index, data)
	if err != nil {
		t.Fatalf("DecodeCharacterSpawnTypes: %v", err)
	}
	want := model.NPCSpawnTypes{
		model.NPCSpawnTypeImportantNPC,
		model.NPCSpawnTypeIntimacy,
		model.NPCSpawnTypeExplorer,
	}
	if !reflect.DeepEqual(got[40029], want) {
		t.Fatalf("spawn types = %v, want %v", got[40029], want)
	}
}

func TestDecodeCharacterSpawnTypesPreservesUnknownType(t *testing.T) {
	index, data := characterSpawnTypeFixture([]spawnTypeTestRow{{key: 1, types: []int{41}}})
	got, err := DecodeCharacterSpawnTypes(index, data)
	if err != nil {
		t.Fatalf("DecodeCharacterSpawnTypes: %v", err)
	}
	want := model.NPCSpawnTypes{model.NPCSpawnTypeUnknown41}
	if !reflect.DeepEqual(got[1], want) {
		t.Fatalf("spawn types = %v, want %v", got[1], want)
	}
}

type spawnTypeTestRow struct {
	key   uint16
	types []int
}

func characterSpawnTypeFixture(rows []spawnTypeTestRow) ([]byte, []byte) {
	index := append([]byte("PABR"), make([]byte, 4)...)
	binary.LittleEndian.PutUint32(index[4:], uint32(len(rows)))
	data := binary.LittleEndian.AppendUint32(nil, uint32(len(rows)))
	for _, row := range rows {
		offset := len(data)
		record := binary.LittleEndian.AppendUint16(nil, row.key)
		flags := make([]byte, 46)
		for _, spawnType := range row.types {
			flags[spawnType] = 1
		}
		record = append(record, flags...)
		data = append(data, record...)

		index = binary.LittleEndian.AppendUint16(index, row.key)
		index = binary.LittleEndian.AppendUint32(index, uint32(offset))
		index = binary.LittleEndian.AppendUint32(index, uint32(len(record)))
	}
	return index, data
}
