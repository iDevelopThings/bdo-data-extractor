package tables

import (
	"encoding/binary"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

func TestDecodeRegionInfoSequentialRecords(t *testing.T) {
	records := [][]byte{
		makeRegionInfoRecord(10, 0, 1, 10, 0x11111111, []uint16{10, 20}, nil),
		makeRegionInfoRecord(20, 1, 1, 10, 0x22222222, nil, [][3]float32{{4, 5, 6}}),
	}
	data := makeRegionInfoTable(records, []string{"Alpha", "Beta"})

	regions, capitals, err := DecodeRegionInfo(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(regions) != 2 {
		t.Fatalf("region count = %d", len(regions))
	}
	if regions[0].Key != 10 || regions[0].Name != "Alpha" || regions[0].Type != 2 {
		t.Fatalf("region 0 = %+v", regions[0])
	}
	if len(regions[0].WarehouseGroup) != 2 || regions[0].WarehouseGroup[1] != 20 {
		t.Fatalf("warehouse group = %v", regions[0].WarehouseGroup)
	}
	if len(regions[1].ExtraPositions) != 1 || regions[1].ExtraPositions[0] != [3]float64{4, 5, 6} {
		t.Fatalf("extra positions = %v", regions[1].ExtraPositions)
	}
	if capitals[1] != 10 {
		t.Fatalf("territory capital = %d", capitals[1])
	}
}

func TestDecodeRegionInfoRejectsBadCountsAndTiling(t *testing.T) {
	record := makeRegionInfoRecord(10, 0, 0, 10, 0, nil, nil)
	badReserved := append([]byte(nil), record...)
	badReserved[88] = 1
	if _, _, err := DecodeRegionInfo(makeRegionInfoTable([][]byte{badReserved}, []string{"Alpha"})); err == nil || !strings.Contains(err.Error(), "nonzero reserved data") {
		t.Fatalf("reserved data error = %v", err)
	}

	badCount := append([]byte(nil), record...)
	binary.LittleEndian.PutUint32(badCount[210:], regionWarehouseLimit+1)
	if _, _, err := DecodeRegionInfo(makeRegionInfoTable([][]byte{badCount}, []string{"Alpha"})); err == nil || !strings.Contains(err.Error(), "warehouse-group count") {
		t.Fatalf("bad warehouse count error = %v", err)
	}

	misaligned := append(record, 0)
	if _, _, err := DecodeRegionInfo(makeRegionInfoTable([][]byte{misaligned}, []string{"Alpha"})); err == nil || !strings.Contains(err.Error(), "leave 1 record bytes") {
		t.Fatalf("misaligned record error = %v", err)
	}
}

func TestDecodeRegionInfoPreservesEveryTypedField(t *testing.T) {
	record := makeRegionInfoRecord(10, 0, 0, 10, 44, nil, nil)
	expected := model.WorldRegionUnknowns{
		Unknown11: 3, Unknown12: true, Unknown13: true,
		Unknown18: true, Unknown19: true, Unknown20: true, Unknown21: true,
		Unknown22: true, Unknown23: true, Unknown24: true, Unknown25: true,
		Unknown26: true, Unknown28: true, Unknown29: 300, Unknown31: true,
		Unknown32: 44, Unknown37: true, Unknown54: true, Unknown55: true,
		Unknown56: true, Unknown57: true, Unknown58: true, Unknown60: 60,
		Unknown66: true, Unknown68: 68, Unknown82: true, Unknown84: 84,
		Unknown107: 107, Unknown115: true, Unknown147: true, Unknown149: 149,
		Unknown153: [5]float64{153, 154, 155, 156, 157},
		Unknown173: 173, Unknown177: 177, Unknown181: 181,
		Unknown185: [6]uint32{185, 186, 187, 188, 189, 190}, Unknown209: true,
		UnknownTail1: 1, UnknownTail3: 3,
		UnknownTail5: [3]float64{5, 6, 7}, UnknownTail17: 17,
		UnknownTail21: 21, UnknownTail22: 22, UnknownTail23: 23, UnknownTail24: true,
		UnknownTail25: [6]float64{25, 26, 27, 28, 29, 30}, UnknownTail49: 49,
		UnknownTail57: 57, UnknownTail61: 61, UnknownTail65: 65,
		UnknownTail69: true, UnknownTail70: true, UnknownTail71: 71,
		UnknownTail75: 75, UnknownTail77: 77, UnknownTail79: 79,
		UnknownTail81: 81, UnknownTail85: [6]uint32{85, 86, 87, 88, 89, 90},
		UnknownTail109: true, UnknownTail110: [3]float64{110, 111, 112},
		UnknownTail122: [3]float64{122, 123, 124}, UnknownTail134: 134,
		UnknownTail135: 135, UnknownTail136: true, UnknownTail137: 137,
		UnknownTail145: 145, UnknownTail154: 154, UnknownTail158: 158,
		UnknownTail162: 162,
	}
	writeRegionInfoUnknowns(record, expected)
	record[2], record[3], record[4] = 10, 20, 30
	record[7] = 6
	record[14], record[15], record[16], record[17], record[27] = 1, 1, 1, 1, 1
	binary.LittleEndian.PutUint32(record[38:], 20)
	writeF32s(record, 42, []float64{42, 43, 44})
	binary.LittleEndian.PutUint16(record[102:], 20)
	binary.LittleEndian.PutUint16(record[104:], 21)
	binary.LittleEndian.PutUint16(record[111:], 22)
	writeF32s(record, 119, []float64{119, 120, 121})
	tail := record[len(record)-regionTailSize:]
	binary.LittleEndian.PutUint16(tail[169:], 40145)

	regions, _, err := DecodeRegionInfo(makeRegionInfoTable([][]byte{record}, []string{"Alpha"}))
	if err != nil {
		t.Fatal(err)
	}
	got := regions[0]
	if !reflect.DeepEqual(got.WorldRegionUnknowns, expected) {
		t.Fatalf("unknown fields mismatch\ngot:  %+v\nwant: %+v", got.WorldRegionUnknowns, expected)
	}
	if got.MapColor != [3]uint8{10, 20, 30} || got.VillageSiegeDay != 6 || !got.Ocean || !got.Desert || !got.Prison || !got.Sea || !got.Locator {
		t.Fatalf("named scalar fields = %+v", got)
	}
	if got.AffiliatedTown == nil || got.RegionGroupKey != 21 || got.Exploration == nil || got.VillainRespawn == nil || got.GuildWharfManager == nil {
		t.Fatalf("named references = %+v", got)
	}
	if got.VillainRespawnPosition != [3]float64{42, 43, 44} || got.WaypointPosition != [3]float64{119, 120, 121} {
		t.Fatalf("named positions = %+v", got)
	}
}

func writeRegionInfoUnknowns(record []byte, value model.WorldRegionUnknowns) {
	record[11] = byte(value.Unknown11)
	for _, offset := range []int{12, 13, 18, 19, 20, 21, 22, 23, 24, 25, 26, 28, 31, 37, 54, 55, 56, 57, 58, 66, 82, 115, 147, 209} {
		record[offset] = 1
	}
	binary.LittleEndian.PutUint16(record[29:], uint16(value.Unknown29))
	binary.LittleEndian.PutUint32(record[32:], value.Unknown32)
	binary.LittleEndian.PutUint32(record[60:], value.Unknown60)
	binary.LittleEndian.PutUint32(record[68:], value.Unknown68)
	binary.LittleEndian.PutUint32(record[84:], value.Unknown84)
	binary.LittleEndian.PutUint16(record[107:], uint16(value.Unknown107))
	binary.LittleEndian.PutUint32(record[149:], value.Unknown149)
	writeF32s(record, 153, value.Unknown153[:])
	binary.LittleEndian.PutUint32(record[173:], value.Unknown173)
	binary.LittleEndian.PutUint32(record[177:], value.Unknown177)
	binary.LittleEndian.PutUint32(record[181:], value.Unknown181)
	for i, field := range value.Unknown185 {
		binary.LittleEndian.PutUint32(record[185+i*4:], field)
	}

	tail := record[len(record)-regionTailSize:]
	binary.LittleEndian.PutUint16(tail[1:], uint16(value.UnknownTail1))
	binary.LittleEndian.PutUint16(tail[3:], uint16(value.UnknownTail3))
	writeF32s(tail, 5, value.UnknownTail5[:])
	binary.LittleEndian.PutUint32(tail[17:], value.UnknownTail17)
	tail[21], tail[22], tail[23], tail[24] = byte(value.UnknownTail21), byte(value.UnknownTail22), byte(value.UnknownTail23), 1
	writeF32s(tail, 25, value.UnknownTail25[:])
	binary.LittleEndian.PutUint64(tail[49:], value.UnknownTail49)
	writeF32s(tail, 57, []float64{value.UnknownTail57, value.UnknownTail61})
	binary.LittleEndian.PutUint32(tail[65:], value.UnknownTail65)
	tail[69], tail[70] = 1, 1
	binary.LittleEndian.PutUint32(tail[71:], value.UnknownTail71)
	binary.LittleEndian.PutUint16(tail[75:], uint16(value.UnknownTail75))
	binary.LittleEndian.PutUint16(tail[77:], uint16(value.UnknownTail77))
	tail[79] = byte(value.UnknownTail79)
	writeF32s(tail, 81, []float64{value.UnknownTail81})
	for i, field := range value.UnknownTail85 {
		binary.LittleEndian.PutUint32(tail[85+i*4:], field)
	}
	tail[109] = 1
	writeF32s(tail, 110, value.UnknownTail110[:])
	writeF32s(tail, 122, value.UnknownTail122[:])
	tail[134], tail[135], tail[136], tail[137] = byte(value.UnknownTail134), byte(value.UnknownTail135), 1, byte(value.UnknownTail137)
	tail[145] = byte(value.UnknownTail145)
	binary.LittleEndian.PutUint32(tail[154:], value.UnknownTail154)
	binary.LittleEndian.PutUint32(tail[158:], value.UnknownTail158)
	binary.LittleEndian.PutUint32(tail[162:], value.UnknownTail162)
}

func writeF32s(data []byte, offset int, values []float64) {
	for i, value := range values {
		binary.LittleEndian.PutUint32(data[offset+i*4:], math.Float32bits(float32(value)))
	}
}

func makeRegionInfoRecord(key, nameIndex, capitalNameIndex, capitalKey int, patchField uint32, warehouses []uint16, extra [][3]float32) []byte {
	record := make([]byte, 389+len(warehouses)*2+len(extra)*12)
	binary.LittleEndian.PutUint16(record, uint16(key))
	record[6] = 2
	record[90] = 1
	binary.LittleEndian.PutUint16(record[92:], uint16(nameIndex))
	binary.LittleEndian.PutUint16(record[96:], uint16(capitalNameIndex))
	binary.LittleEndian.PutUint16(record[100:], uint16(capitalKey))
	for i, value := range []float32{1, 2, 3} {
		binary.LittleEndian.PutUint32(record[131+i*4:], math.Float32bits(value))
	}
	binary.LittleEndian.PutUint32(record[149:], patchField)
	binary.LittleEndian.PutUint32(record[210:], uint32(len(warehouses)))
	positionCountOffset := 214 + len(warehouses)*2
	for i, warehouse := range warehouses {
		binary.LittleEndian.PutUint16(record[214+i*2:], warehouse)
	}
	binary.LittleEndian.PutUint32(record[positionCountOffset:], uint32(len(extra)))
	for i, position := range extra {
		for axis, value := range position {
			binary.LittleEndian.PutUint32(record[positionCountOffset+4+i*12+axis*4:], math.Float32bits(value))
		}
	}
	return record
}

func makeRegionInfoTable(records [][]byte, values []string) []byte {
	data := []byte{'P', 'A', 'B', 'R'}
	data = binary.LittleEndian.AppendUint32(data, uint32(len(records)))
	for _, record := range records {
		data = append(data, record...)
	}
	stringTablePosition := len(data)
	data = binary.LittleEndian.AppendUint32(data, uint32(len(values)))
	data = append(data, 1)
	for _, value := range values {
		encoded := make([]byte, 0, len(value)*2)
		for _, character := range value {
			encoded = binary.LittleEndian.AppendUint16(encoded, uint16(character))
		}
		data = binary.LittleEndian.AppendUint32(data, uint32(len(encoded)))
		data = append(data, encoded...)
		data = append(data, 1)
	}
	data = binary.LittleEndian.AppendUint64(data, uint64(stringTablePosition))
	return data
}
