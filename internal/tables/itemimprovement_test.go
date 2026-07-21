package tables

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeItemImprovements(t *testing.T) {
	// One 41B reform row (Kharazad Necklace → Dawnbound) + one 25B alt skipped.
	reform := make([]byte, 41)
	binary.LittleEndian.PutUint32(reform[0:], 5)
	binary.LittleEndian.PutUint32(reform[8:], 11698)
	for i := 0; i < 4; i++ {
		binary.LittleEndian.PutUint32(reform[12+i*4:], 11697)
	}
	binary.LittleEndian.PutUint32(reform[28:], 3328)

	alt := make([]byte, 25)
	binary.LittleEndian.PutUint32(alt[0:], 1)
	binary.LittleEndian.PutUint32(alt[8:], 695166)
	binary.LittleEndian.PutUint32(alt[12:], 1024)

	data := appendU32(nil, 2)
	offReform := uint32(len(data))
	data = append(data, reform...)
	offAlt := uint32(len(data))
	data = append(data, alt...)

	offset := appendU32(nil, 2)
	offset = appendIndexEntry(offset, 4233, offReform, 41)
	offset = appendIndexEntry(offset, 759, offAlt, 25)

	rows, altCount, err := DecodeItemImprovements(offset, data)
	if err != nil {
		t.Fatal(err)
	}
	if altCount != 1 {
		t.Fatalf("altCount = %d, want 1", altCount)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	r := rows[0]
	if r.Key != 4233 || r.Result != 11698 || r.Flag != 3328 {
		t.Fatalf("row = %#v", r)
	}
	if len(r.Bases) != 1 || r.Bases[0] != 11697 {
		t.Fatalf("bases = %#v", r.Bases)
	}
}

func TestDecodeItemImprovementsLive(t *testing.T) {
	root := filepath.Join("..", "..", ".tmp", "probe", "gamecommondata", "binary")
	data, err := os.ReadFile(filepath.Join(root, "itemimprovement.dbss"))
	if err != nil {
		t.Skip(err)
	}
	offsetRaw, err := os.ReadFile(filepath.Join(root, "itemimprovementoffset.dbss"))
	if err != nil {
		t.Skip(err)
	}
	rows, altCount, err := DecodeItemImprovements(offsetRaw, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3736 || altCount != 87 {
		t.Fatalf("rows=%d alt=%d, want 3736/87", len(rows), altCount)
	}
	found := false
	for _, r := range rows {
		if r.Result == 11698 && len(r.Bases) == 1 && r.Bases[0] == 11697 && r.Flag == 3328 {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("missing Kharazad Necklace → Dawnbound row")
	}
}

func appendIndexEntry(b []byte, key, offset, size uint32) []byte {
	b = appendU32(b, key)
	b = appendU32(b, offset)
	b = appendU32(b, size)
	return b
}
