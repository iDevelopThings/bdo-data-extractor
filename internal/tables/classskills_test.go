package tables

import (
	"encoding/binary"
	"testing"
)

func TestDecodeSkillGroups(t *testing.T) {
	data := appendU32(nil, 2)
	data = appendU16(data, 100)
	data = appendU32(data, 3)
	data = appendU32(data, 0)
	data = appendU32(data, 0x12340001)
	data = appendU32(data, 0x12350001)
	data = appendU16(data, 200)
	data = appendU32(data, 2)
	data = appendU32(data, 0)
	data = appendU32(data, 0x23450001)

	groups, err := DecodeSkillGroups(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := groups[100].SkillKeys; len(got) != 3 || got[2] != 0x12350001 {
		t.Fatalf("group 100 ranks = %#v", got)
	}
	if got := groups[200].SkillKeys; len(got) != 2 || got[1] != 0x23450001 {
		t.Fatalf("group 200 ranks = %#v", got)
	}
}

func TestDecodeSkillTrees(t *testing.T) {
	record := []byte{21}
	record = appendU32(record, 2)
	record = appendU32(record, 1)
	// A line-only cell with three drawing types.
	record = appendU32(record, 3)
	record = append(record, 4, 7, 12)
	record = appendU32(record, 0)
	// A skill cell: group 3552, unknown middle byte 9, subgroup 2.
	record = appendU32(record, 2)
	record = append(record, 2, 12)
	record = appendU32(record, uint32(3552)|(9<<16)|(2<<24))

	table, err := DecodeSkillTrees(testPABR(1, record), SkillTreeCombat)
	if err != nil {
		t.Fatal(err)
	}
	if len(table.Trees) != 1 || table.Trees[0].ClassType != 21 || table.Trees[0].Width != 2 || table.Trees[0].Height != 1 {
		t.Fatalf("tree = %#v", table.Trees)
	}
	cell := table.Trees[0].Cells[1]
	if !cell.HasSkillGroup() || cell.Group != 3552 || cell.Unknown16 != 9 || cell.SubGroup != 2 || cell.X != 1 || cell.Y != 0 {
		t.Fatalf("skill cell = %#v", cell)
	}
}

func testPABR(rows uint32, records []byte) []byte {
	data := append([]byte("PABR"), 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(data[4:8], rows)
	data = append(data, records...)
	var pointer [8]byte
	binary.LittleEndian.PutUint64(pointer[:], uint64(len(data)))
	return append(data, pointer[:]...)
}
