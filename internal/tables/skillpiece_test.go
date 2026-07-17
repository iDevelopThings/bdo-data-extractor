package tables

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

func TestDecodeItemSets(t *testing.T) {
	record := make([]byte, 0)
	record = appendU32(record, 57993)
	record = appendU32(record, 2)
	record = appendU32(record, 2)
	record = appendU16(record, 1)
	record = appendSkillPieceUTF16(record, "Manos/Preonne")
	record = appendSkillPieceUTF16(record, "2-piece")
	record = appendSkillPieceUTF16(record, "Life EXP +5%")
	record = appendU32(record, 4)
	record = appendU16(record, 2)
	record = appendSkillPieceUTF16(record, "Manos/Preonne")
	record = appendSkillPieceUTF16(record, "4-piece")
	record = appendSkillPieceUTF16(record, "Life EXP +10%")
	record = appendU32(record, 0)

	offset := appendU32(nil, 1)
	offset = appendU16(offset, 57993)
	offset = appendU32(offset, 0)
	offset = appendU32(offset, uint32(len(record)))

	sets, err := DecodeItemSets(offset, record)
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != 1 || sets[0].SkillNo != 57993 {
		t.Fatalf("sets = %#v", sets)
	}
	if got := sets[0].Bonuses; len(got) != 2 || got[0].Pieces != 2 || got[1].Apply != 2 || got[1].Description != "Life EXP +10%" {
		t.Fatalf("bonuses = %#v", got)
	}
}

func TestDecodeItemSetsRejectsNonzeroFooter(t *testing.T) {
	record := appendU32(nil, 50068)
	record = appendU32(record, 1)
	record = appendU32(record, 1)
	record = appendU16(record, 1)
	record = appendSkillPieceUTF16(record, "")
	record = appendSkillPieceUTF16(record, "")
	record = appendSkillPieceUTF16(record, "")
	record = appendU32(record, 7)

	offset := appendU32(nil, 1)
	offset = appendU16(offset, 50068)
	offset = appendU32(offset, 0)
	offset = appendU32(offset, uint32(len(record)))

	if _, err := DecodeItemSets(offset, record); err == nil {
		t.Fatal("DecodeItemSets accepted a nonzero footer")
	}
}

func appendU16(dst []byte, value uint16) []byte {
	return binary.LittleEndian.AppendUint16(dst, value)
}

func appendU32(dst []byte, value uint32) []byte {
	return binary.LittleEndian.AppendUint32(dst, value)
}

func appendSkillPieceUTF16(dst []byte, value string) []byte {
	units := utf16.Encode([]rune(value))
	dst = binary.LittleEndian.AppendUint64(dst, uint64(len(units)))
	for _, unit := range units {
		dst = binary.LittleEndian.AppendUint16(dst, unit)
	}
	return dst
}
