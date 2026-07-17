package tables

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestDecodeCaphrasPreservesStatsAndValuedEffects(t *testing.T) {
	record := make([]byte, 0, 86)
	record = appendU32(record, 1)
	record = appendU32(record, 2)
	record = appendCaphrasEntry(record, 7, 18, 10, [7]float32{1, 2, 3, 4, 5, 6, 20}, 8)
	record = appendCaphrasEntry(record, 7, 18, 25, [7]float32{2, 3, 4, 5, 6, 7, 40}, 10)

	categories, err := DecodeCaphras(caphrasPABR(record))
	if err != nil {
		t.Fatal(err)
	}
	if len(categories) != 1 || categories[0].Key != 7 {
		t.Fatalf("categories = %#v", categories)
	}
	level := categories[0].Levels[0]
	if level.EnchantLevel != 18 || len(level.Steps) != 2 {
		t.Fatalf("level = %#v", level)
	}
	step := level.Steps[1]
	if step.Stones != 15 || step.TotalStones != 25 {
		t.Fatalf("stone costs = %d/%d", step.Stones, step.TotalStones)
	}
	if step.Stats.Evasion != 4 || step.Stats.DamageReduction != 6 || step.Stats.MaxHP != 40 {
		t.Fatalf("stats = %#v", step.Stats)
	}
	evasion := findEffect(t, step.Effects, "ALL_EVA_INCRE")
	if evasion.Value != 4 || evasion.Op != "+" || len(evasion.CurveFields) != 0 {
		t.Fatalf("valued evasion effect = %#v", evasion)
	}
	damageReduction := findEffect(t, step.Effects, "ALL_DAM_REDUCE_INCRE")
	if damageReduction.Value != 6 || damageReduction.Op != "+" || len(damageReduction.CurveFields) != 0 {
		t.Fatalf("valued damage-reduction effect = %#v", damageReduction)
	}
}

func TestDecodeCaphrasRejectsTrailingBytes(t *testing.T) {
	record := appendU32(nil, 1)
	record = appendU32(record, 1)
	record = appendCaphrasEntry(record, 1, 18, 10, [7]float32{}, 0)
	record = append(record, 0)
	if _, err := DecodeCaphras(caphrasPABR(record)); err == nil {
		t.Fatal("expected trailing-byte error")
	}
}

func appendCaphrasEntry(dst []byte, key, level byte, total uint32, stats [7]float32, maxMP uint32) []byte {
	dst = append(dst, key, 0, level)
	dst = appendU32(dst, total)
	for _, value := range stats {
		dst = appendU32(dst, math.Float32bits(value))
	}
	dst = appendU32(dst, maxMP)
	return dst
}

func caphrasPABR(record []byte) []byte {
	data := append([]byte("PABR"), 1, 0, 0, 0)
	data = append(data, record...)
	var pointer [8]byte
	binary.LittleEndian.PutUint64(pointer[:], uint64(len(data)))
	return append(data, pointer[:]...)
}
