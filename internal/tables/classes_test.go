package tables

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestDecodeClassAvailability(t *testing.T) {
	data := []byte{'P', 'A', 'B', 'R', 2, 0, 0, 0}
	data = append(data, 4, 0, 0, 0, 0, 1, 7, 0, 0, 0)
	data = append(data, 14, 1, 0, 0, 0, 0, 0, 0, 0, 0)
	stringTablePos := len(data)
	data = binary.LittleEndian.AppendUint32(data, 2)
	data = append(data, 1)
	for _, value := range []string{"Ranger", "Reserved"} {
		encoded := make([]byte, 0, len(value)*2)
		for _, r := range value {
			encoded = binary.LittleEndian.AppendUint16(encoded, uint16(r))
		}
		data = binary.LittleEndian.AppendUint32(data, uint32(len(encoded)))
		data = append(data, encoded...)
		data = append(data, 1)
	}
	data = binary.LittleEndian.AppendUint64(data, uint64(stringTablePos))

	classes, err := DecodeClassAvailability(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(classes) != 2 || !classes[0].Playable || classes[1].Playable {
		t.Fatalf("classes = %+v", classes)
	}
	if classes[0].SourceName != "Ranger" || classes[1].SourceName != "Reserved" {
		t.Fatalf("classes = %+v", classes)
	}
	if classes[0].Unknown6 != 7 {
		t.Fatalf("unknown6 = %d", classes[0].Unknown6)
	}
}

func TestDecodeClassGrowth(t *testing.T) {
	record := []byte{4, 4, 2, 0}
	record = append(record, make([]byte, 11)...)
	binary.LittleEndian.PutUint16(record[4:], 5)
	binary.LittleEndian.PutUint16(record[6:], 6)
	binary.LittleEndian.PutUint16(record[8:], 7)
	binary.LittleEndian.PutUint32(record[10:], 8)
	record[14] = 9
	record = binary.LittleEndian.AppendUint32(record, 2)
	record = binary.LittleEndian.AppendUint32(record, 10201)
	record = binary.LittleEndian.AppendUint32(record, 10301)
	record = append(record, make([]byte, 99)...)
	record[len(record)-5] = 3
	for _, value := range []string{"Ranger", "Description", "ranger.webm"} {
		record = appendTestUTF16(record, value)
	}
	record = append(record, 1)
	for _, value := range []string{"consume1", "consume2", "consume3", "consume4"} {
		record = appendTestUTF8(record, value)
	}
	for range 6 {
		record = binary.LittleEndian.AppendUint32(record, math.Float32bits(1))
	}
	record = append(record, 2)
	for _, skill := range []uint32{10201, 10301, 14731} {
		record = binary.LittleEndian.AppendUint32(record, skill)
	}
	record = binary.LittleEndian.AppendUint16(record, 1)
	for range 7 {
		record = binary.LittleEndian.AppendUint32(record, math.Float32bits(2))
	}
	record = binary.LittleEndian.AppendUint32(record, 0)
	for range 2 {
		record = binary.LittleEndian.AppendUint32(record, 1)
		record = append(record, 1)
		record = appendTestUTF8(record, "weapon.pac")
	}
	data := binary.LittleEndian.AppendUint32(nil, 1)
	data = append(data, record...)
	index := binary.LittleEndian.AppendUint32(nil, 1)
	index = append(index, 4)
	index = binary.LittleEndian.AppendUint32(index, 5)
	index = binary.LittleEndian.AppendUint32(index, uint32(len(record)-1))

	classes, err := DecodeClassGrowth(index, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(classes) != 1 || classes[0].ClassType != 4 || classes[0].CharacterKey != 2 {
		t.Fatalf("classes = %+v", classes)
	}
	if classes[0].SelectionMovie != "ranger.webm" {
		t.Fatalf("selection movie = %q", classes[0].SelectionMovie)
	}
	if classes[0].Gender != "female" || classes[0].PreviewWeapons[2] != 14731 {
		t.Fatalf("class details = %+v", classes[0])
	}
	if len(classes[0].StarterWeapons) != 2 || classes[0].StarterWeapons[0] != 10201 || classes[0].StarterWeapons[1] != 10301 {
		t.Fatalf("starter weapons = %v", classes[0].StarterWeapons)
	}
	if classes[0].Unknowns.Unknown4 != 5 || classes[0].Unknowns.Unknown10 != 8 || classes[0].Unknowns.Unknown14 != 9 {
		t.Fatalf("class header unknowns = %+v", classes[0].Unknowns)
	}
	if classes[0].Unknowns.UnknownConfiguration94 != 3 {
		t.Fatalf("configuration unknowns = %+v", classes[0].Unknowns)
	}
	if len(classes[0].WeaponAssetSets) != 2 || classes[0].WeaponAssetSets[0][0].Path != "weapon.pac" {
		t.Fatalf("weapon assets = %+v", classes[0].WeaponAssetSets)
	}
}

func appendTestUTF16(dst []byte, value string) []byte {
	dst = binary.LittleEndian.AppendUint64(dst, uint64(len(value)))
	for _, r := range value {
		dst = binary.LittleEndian.AppendUint16(dst, uint16(r))
	}
	return dst
}

func appendTestUTF8(dst []byte, value string) []byte {
	dst = binary.LittleEndian.AppendUint64(dst, uint64(len(value)))
	return append(dst, value...)
}

func TestDecodeCharacterLevelRules(t *testing.T) {
	data := []byte{'P', 'A', 'B', 'R'}
	for classType := range 37 {
		data = binary.LittleEndian.AppendUint32(data, experienceLevelCount)
		for level := range experienceLevelCount {
			record := make([]byte, experienceRecordSize)
			binary.LittleEndian.PutUint32(record, uint32(classType))
			binary.LittleEndian.PutUint32(record[4:], uint32(level))
			binary.LittleEndian.PutUint32(record[32:], math.Float32bits(1))
			binary.LittleEndian.PutUint32(record[36:], 1)
			binary.LittleEndian.PutUint32(record[60:], math.Float32bits(1))
			binary.LittleEndian.PutUint32(record[108:], math.Float32bits(1))
			binary.LittleEndian.PutUint32(record[140:], math.Float32bits(1))
			if level > 0 {
				for _, offset := range []int{88, 92, 96, 128, 132, 136} {
					binary.LittleEndian.PutUint32(record[offset:], 5)
				}
			}
			if level >= 60 {
				binary.LittleEndian.PutUint32(record[144:], math.Float32bits(1))
			}
			if level >= 56 {
				binary.LittleEndian.PutUint32(record[148:], math.Float32bits(1))
			}
			data = append(data, record...)
		}
	}
	data = append(data, make([]byte, experienceFooterSize)...)
	stringTablePos := len(data)
	data = binary.LittleEndian.AppendUint32(data, 0)
	data = binary.LittleEndian.AppendUint64(data, uint64(stringTablePos))

	rules, err := DecodeCharacterLevelRules(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 37*experienceLevelCount {
		t.Fatalf("rule count = %d", len(rules))
	}
	level56 := rules[4*experienceLevelCount+56]
	if level56.ClassType != 4 || level56.Level != 56 || level56.APBonus != 0 || level56.DPBonus != 1 {
		t.Fatalf("class 4 level 56 = %+v", level56)
	}
	level60 := rules[4*experienceLevelCount+60]
	if level60.APBonus != 1 || level60.DPBonus != 1 {
		t.Fatalf("class 4 level 60 = %+v", level60)
	}
	if level60.UnknownStat4 != 1 || level60.UnknownStat8 != 1 || level60.UnknownStat32 != 1 || level60.UnknownStat80 != 1 || level60.UnknownStat112 != 1 {
		t.Fatalf("class 4 level 60 scalar stat unknowns = %+v", level60.CharacterLevelStat)
	}
	if level60.UnknownStat60 != 5 || level60.UnknownStat64 != 5 || level60.UnknownStat68 != 5 || level60.UnknownStat100 != 5 || level60.UnknownStat104 != 5 || level60.UnknownStat108 != 5 {
		t.Fatalf("class 4 level 60 triad stat unknowns = %+v", level60.CharacterLevelStat)
	}

	data[16] = 1
	if _, err := DecodeCharacterLevelRules(data); err == nil {
		t.Fatal("nonzero reserved level-rule byte was accepted")
	}
}

func TestDecodeFitnessLevels(t *testing.T) {
	data := binary.LittleEndian.AppendUint32(nil, 3)
	index := make([]byte, 0)
	for kind := range 3 {
		data = binary.LittleEndian.AppendUint32(data, 2)
		index = binary.LittleEndian.AppendUint32(index, 2)
		for level := range 2 {
			offset := len(data)
			record := make([]byte, 29)
			record[0] = byte(kind)
			binary.LittleEndian.PutUint32(record[1:], uint32(level))
			binary.LittleEndian.PutUint32(record[5:], uint32(100+level))
			binary.LittleEndian.PutUint32(record[9:], math.Float32bits(float32(level)))
			binary.LittleEndian.PutUint32(record[13:], math.Float32bits(float32(25*level)))
			binary.LittleEndian.PutUint32(record[17:], math.Float32bits(float32(20_000*level)))
			binary.LittleEndian.PutUint32(record[21:], math.Float32bits(float32(10*level)))
			binary.LittleEndian.PutUint32(record[25:], math.Float32bits(float32(10*level)))
			data = append(data, record...)
			index = binary.LittleEndian.AppendUint32(index, uint32(level))
			index = binary.LittleEndian.AppendUint32(index, uint32(offset))
			index = binary.LittleEndian.AppendUint32(index, uint32(len(record)))
		}
	}

	curves, err := DecodeFitnessLevels(index, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(curves[0]) != 2 || curves[0][1].MaxStamina != 25 {
		t.Fatalf("breath = %+v", curves[0])
	}
	if curves[1][1].MaxWeightLT != 2 {
		t.Fatalf("strength = %+v", curves[1])
	}
	if curves[2][1].MaxHP != 10 || curves[2][1].MaxMP != 10 {
		t.Fatalf("health = %+v", curves[2])
	}
	if curves[2][1].Unknown9 != 1 {
		t.Fatalf("health unknown9 = %v", curves[2][1].Unknown9)
	}
}
