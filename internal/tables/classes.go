package tables

import (
	"fmt"
	"sort"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

// ClassAvailability is one pcgrowthsimply.bss row.
type ClassAvailability struct {
	ClassType  model.CharacterClassType
	SourceName string
	Playable   bool
	Unknown6   uint32
}

// ClassGrowth is one fully consumed pcgrowth.dbss record. Unidentified spans
// remain available through Unknowns instead of being discarded.
type ClassGrowth struct {
	ClassType         model.CharacterClassType
	CharacterKey      uint32
	SourceName        string
	SourceDescription string
	Gender            model.CharacterGender
	StarterWeapons    []uint32
	PreviewWeapons    []uint32
	SelectionMovie    string
	ConsumeAnimations []string
	WeaponAssetSets   [][]model.ClassWeaponAsset
	Unknowns          model.CharacterClassUnknowns
}

// DecodeClassAvailability reads every field of pcgrowthsimply.bss.
func DecodeClassAvailability(data []byte) ([]ClassAvailability, error) {
	pabr, err := bss.OpenPABR(data)
	if err != nil {
		return nil, fmt.Errorf("pcgrowthsimply: %w", err)
	}
	recordSize, fixed := pabr.RecordSize()
	if !fixed || recordSize != 10 {
		return nil, fmt.Errorf("pcgrowthsimply: record size %d, want 10", recordSize)
	}
	strings := bss.ReadUTF16StringTable(data, pabr.StringTablePos)
	if len(strings) == 0 {
		return nil, fmt.Errorf("pcgrowthsimply: empty string table")
	}

	out := make([]ClassAvailability, 0, pabr.Rows)
	for i := range pabr.Rows {
		o := pabr.RecordsStart + i*recordSize
		c := bss.NewCursor(data, o, o+recordSize)
		classType := model.CharacterClassType(c.U8())
		nameIndex := int(c.U32())
		playable := c.Bool()
		unknown6 := c.U32()
		if !c.OK() || c.Remaining() != 0 {
			return nil, fmt.Errorf("pcgrowthsimply: class %d did not consume its 10-byte row", classType)
		}
		if nameIndex < 0 || nameIndex >= len(strings) {
			return nil, fmt.Errorf("pcgrowthsimply: class %d has string index %d, table has %d", classType, nameIndex, len(strings))
		}
		out = append(out, ClassAvailability{
			ClassType:  classType,
			SourceName: strings[nameIndex],
			Playable:   playable,
			Unknown6:   unknown6,
		})
	}
	return out, nil
}

// DecodeClassGrowth reads pcgrowthoffset.dbss and consumes every byte of each
// pcgrowth.dbss record.
func DecodeClassGrowth(offsetData, data []byte) ([]ClassGrowth, error) {
	entries, err := bss.ParseU8OneBasedOffsetIndex("pcgrowth", offsetData, len(data))
	if err != nil {
		return nil, err
	}
	declared := int(bss.U32(offsetData, 0))
	if len(offsetData) != 4+declared*9 || len(entries) != declared {
		return nil, fmt.Errorf("pcgrowth: index count/size mismatch: count=%d entries=%d bytes=%d", declared, len(entries), len(offsetData))
	}
	if len(data) < 4 || int(bss.U32(data, 0)) != declared {
		return nil, fmt.Errorf("pcgrowth: data count does not match index count %d", declared)
	}
	ordered := append([]bss.IndexEntry(nil), entries...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Offset < ordered[j].Offset
	})
	expectedOffset := uint32(4)
	keys := make(map[uint32]bool, len(entries))
	for _, entry := range ordered {
		if entry.Offset != expectedOffset {
			return nil, fmt.Errorf("pcgrowth: class %d starts at %d, want %d", entry.Key, entry.Offset, expectedOffset)
		}
		if keys[entry.Key] {
			return nil, fmt.Errorf("pcgrowth: duplicate class key %d", entry.Key)
		}
		keys[entry.Key] = true
		expectedOffset += entry.Size
	}
	if int(expectedOffset) != len(data) {
		return nil, fmt.Errorf("pcgrowth: records end at %d, data ends at %d", expectedOffset, len(data))
	}

	out := make([]ClassGrowth, 0, len(entries))
	for _, entry := range entries {
		record, ok := entry.Slice(data)
		if !ok || len(record) < 19 {
			return nil, fmt.Errorf("pcgrowth: class %d has a truncated record", entry.Key)
		}
		growth, err := decodeClassGrowthRecord(record, model.CharacterClassType(entry.Key))
		if err != nil {
			return nil, err
		}
		out = append(out, growth)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ClassType < out[j].ClassType
	})
	return out, nil
}

func decodeClassGrowthRecord(record []byte, indexClassType model.CharacterClassType) (ClassGrowth, error) {
	c := bss.NewCursor(record, 0, len(record))
	classType := model.CharacterClassType(c.U8())
	duplicateClassType := model.CharacterClassType(c.U8())
	characterKey := c.U16()
	unknowns := model.CharacterClassUnknowns{
		Unknown4:  c.U16(),
		Unknown6:  c.U16(),
		Unknown8:  c.U16(),
		Unknown10: c.U32(),
		Unknown14: c.U8(),
	}
	starterWeapons := c.U32List(64)
	configuration := c.U8N(99)
	if len(configuration) == 99 {
		unknowns.UnknownConfiguration0 = configuration[:94]
		unknowns.UnknownConfiguration94 = int(configuration[94])
		unknowns.UnknownConfiguration95 = configuration[95:]
	}
	sourceName := c.UTF16()
	sourceDescription := c.UTF16()
	selectionMovie := c.UTF16()
	genderFlag := c.U8()
	consumeAnimations := make([]string, 4)
	for i := range consumeAnimations {
		consumeAnimations[i] = c.UTF8()
	}
	unknowns.UnknownPresentation0 = readFloatArray(c, 6)
	unknowns.UnknownPresentation24 = c.U8()
	previewWeapons := c.U32N(3)
	unknowns.UnknownPresentation37 = c.U16()
	unknowns.UnknownPresentation39 = readFloatArray(c, 7)
	unknowns.UnknownPresentation67 = c.U32()
	if classType == 17 {
		unknowns.UnknownPresentationExtra = c.U32N(4)
	}

	weaponAssetSets := make([][]model.ClassWeaponAsset, 2)
	for set := range weaponAssetSets {
		count := int(c.U32())
		if count < 0 || count > 16 {
			return ClassGrowth{}, fmt.Errorf("pcgrowth: class %d weapon asset set %d has bad count %d", classType, set, count)
		}
		assets := make([]model.ClassWeaponAsset, count)
		for i := range assets {
			assets[i] = model.ClassWeaponAsset{Slot: c.U8(), Path: c.UTF8()}
		}
		weaponAssetSets[set] = assets
	}
	if !c.OK() || c.Remaining() != 0 {
		return ClassGrowth{}, fmt.Errorf("pcgrowth: class %d leaves %d bytes after decode", classType, c.Remaining())
	}
	if classType != indexClassType || duplicateClassType != classType {
		return ClassGrowth{}, fmt.Errorf("pcgrowth: index class %d resolves to class bytes %d/%d", indexClassType, classType, duplicateClassType)
	}

	var gender model.CharacterGender
	switch genderFlag {
	case 0:
		gender = model.CharacterGenderMale
	case 1:
		gender = model.CharacterGenderFemale
	default:
		return ClassGrowth{}, fmt.Errorf("pcgrowth: class %d has unknown gender flag %d", classType, genderFlag)
	}
	return ClassGrowth{
		ClassType:         classType,
		CharacterKey:      characterKey,
		SourceName:        sourceName,
		SourceDescription: sourceDescription,
		Gender:            gender,
		StarterWeapons:    starterWeapons,
		PreviewWeapons:    previewWeapons,
		SelectionMovie:    selectionMovie,
		ConsumeAnimations: consumeAnimations,
		WeaponAssetSets:   weaponAssetSets,
		Unknowns:          unknowns,
	}, nil
}

func readFloatArray(c *bss.Cursor, count int) []float64 {
	out := make([]float64, count)
	for i := range out {
		out[i] = c.F32()
	}
	return out
}

const (
	experienceLevelCount = 131
	experienceRecordSize = 228
	experienceFooterSize = 200
)

// DecodeCharacterLevelRules reads every class and level record in experience.bss.
func DecodeCharacterLevelRules(data []byte) ([]model.CharacterLevelRule, error) {
	pabr, err := bss.OpenPABR(data)
	if err != nil {
		return nil, fmt.Errorf("experience: %w", err)
	}
	if pabr.Rows != experienceLevelCount {
		return nil, fmt.Errorf("experience: first class has %d levels, want %d", pabr.Rows, experienceLevelCount)
	}

	groupSize := 4 + experienceLevelCount*experienceRecordSize
	recordAreaSize := pabr.StringTablePos - 4 - experienceFooterSize
	if recordAreaSize <= 0 || recordAreaSize%groupSize != 0 {
		return nil, fmt.Errorf("experience: %d record-area bytes do not tile into %d-byte class groups", recordAreaSize, groupSize)
	}
	classCount := recordAreaSize / groupSize
	out := make([]model.CharacterLevelRule, 0, classCount*experienceLevelCount)
	pos := 4
	for classType := range classCount {
		levelCount := int(bss.U32(data, pos))
		pos += 4
		if levelCount != experienceLevelCount {
			return nil, fmt.Errorf("experience: class %d has %d levels, want %d", classType, levelCount, experienceLevelCount)
		}
		for level := range levelCount {
			c := bss.NewCursor(data, pos, pos+experienceRecordSize)
			recordClassType := model.CharacterClassType(c.U32())
			recordLevel := int(c.U32())
			reservedOK := c.Zero(24)
			rule := model.CharacterLevelRule{
				ClassType: recordClassType,
				Level:     recordLevel,
				CharacterLevelStat: model.CharacterLevelStat{
					UnknownStat4: c.F32(),
					UnknownStat8: c.U32(),
				},
			}
			reservedOK = c.Zero(20) && reservedOK
			rule.UnknownStat32 = c.F32()
			reservedOK = c.Zero(24) && reservedOK
			rule.UnknownStat60 = c.U32()
			rule.UnknownStat64 = c.U32()
			rule.UnknownStat68 = c.U32()
			reservedOK = c.Zero(8) && reservedOK
			rule.UnknownStat80 = c.F32()
			reservedOK = c.Zero(16) && reservedOK
			rule.UnknownStat100 = c.U32()
			rule.UnknownStat104 = c.U32()
			rule.UnknownStat108 = c.U32()
			rule.UnknownStat112 = c.F32()
			rule.APBonus = c.F32()
			rule.DPBonus = c.F32()
			reservedOK = c.Zero(76) && reservedOK
			if !c.OK() || c.Remaining() != 0 {
				return nil, fmt.Errorf("experience: class %d level %d did not consume its %d-byte record", classType, level, experienceRecordSize)
			}
			if !reservedOK {
				return nil, fmt.Errorf("experience: class %d level %d has nonzero reserved character-stat bytes", classType, level)
			}
			if recordClassType != model.CharacterClassType(classType) || recordLevel != level {
				return nil, fmt.Errorf("experience: class/level %d/%d resolves to %d/%d", classType, level, recordClassType, recordLevel)
			}
			out = append(out, rule)
			pos += experienceRecordSize
		}
	}
	if pos+experienceFooterSize != pabr.StringTablePos {
		return nil, fmt.Errorf("experience: records end at %d, footer ends at %d", pos, pabr.StringTablePos)
	}
	if !bss.AllZero(data[pos:pabr.StringTablePos]) {
		return nil, fmt.Errorf("experience: nonzero byte in %d-byte footer", experienceFooterSize)
	}
	return out, nil
}

// DecodeFitnessLevels reads the complete 29-byte records referenced by the
// three concatenated indexes in fitnessleveloffset.dbss.
func DecodeFitnessLevels(offsetData, data []byte) ([3][]model.FitnessLevel, error) {
	var out [3][]model.FitnessLevel
	if len(data) < 4 || len(offsetData) < 4 {
		return out, fmt.Errorf("fitnesslevel: truncated table")
	}
	kindCount := int(bss.U32(data, 0))
	if kindCount != len(out) {
		return out, fmt.Errorf("fitnesslevel: kind count %d, want %d", kindCount, len(out))
	}

	indexPos := 0
	dataPos := 4
	for kind := range kindCount {
		if indexPos+4 > len(offsetData) || dataPos+4 > len(data) {
			return out, fmt.Errorf("fitnesslevel: missing index or data group for kind %d", kind)
		}
		count := int(bss.U32(offsetData, indexPos))
		dataCount := int(bss.U32(data, dataPos))
		dataPos += 4
		indexSize := 4 + count*12
		if count <= 0 || indexPos+indexSize > len(offsetData) {
			return out, fmt.Errorf("fitnesslevel: bad index count %d for kind %d", count, kind)
		}
		if dataCount != count {
			return out, fmt.Errorf("fitnesslevel: kind %d index count %d differs from data count %d", kind, count, dataCount)
		}
		entries, err := bss.ParseOffsetIndex(offsetData[indexPos:indexPos+indexSize], len(data))
		if err != nil {
			return out, fmt.Errorf("fitnesslevel kind %d: %w", kind, err)
		}
		indexPos += indexSize

		levels := make([]model.FitnessLevel, 0, len(entries))
		for i, entry := range entries {
			record, ok := entry.Slice(data)
			if !ok || len(record) != 29 {
				return out, fmt.Errorf("fitnesslevel: kind %d level %d has record size %d, want 29", kind, entry.Key, len(record))
			}
			if int(entry.Offset) != dataPos+i*29 {
				return out, fmt.Errorf("fitnesslevel: kind %d level %d starts at %d, want %d", kind, entry.Key, entry.Offset, dataPos+i*29)
			}
			level := int(bss.U32(record, 1))
			if int(record[0]) != kind || uint32(level) != entry.Key {
				return out, fmt.Errorf("fitnesslevel: index kind/level %d/%d resolves to %d/%d", kind, entry.Key, record[0], level)
			}
			levels = append(levels, model.FitnessLevel{
				Level:              level,
				RequiredExperience: bss.U32(record, 5),
				Unknown9:           bss.F32(record, 9),
				MaxStamina:         bss.F32(record, 13),
				MaxWeightLT:        bss.F32(record, 17) / 10_000,
				MaxHP:              bss.F32(record, 21),
				MaxMP:              bss.F32(record, 25),
			})
		}
		dataPos += count * 29
		sort.Slice(levels, func(i, j int) bool {
			return levels[i].Level < levels[j].Level
		})
		out[kind] = levels
	}
	if indexPos != len(offsetData) || dataPos != len(data) {
		return out, fmt.Errorf("fitnesslevel: trailing bytes (index %d, data %d)", len(offsetData)-indexPos, len(data)-dataPos)
	}
	return out, nil
}
