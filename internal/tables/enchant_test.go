package tables

import (
	"encoding/binary"
	"math"
	"testing"
	"unicode/utf16"

	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

func TestDecodeEnchantRowConsumesBothDescriptionsAndFooter(t *testing.T) {
	record := enchantRecordFixture(1234)
	level, err := decodeEnchantRow(record, 5<<24|1234)
	if err != nil {
		t.Fatal(err)
	}

	if level.Level != 5 || level.SourceDescription != "custom enhancement text" {
		t.Fatalf("identity/description = level %d, %q", level.Level, level.SourceDescription)
	}
	if level.ApMin != 30 || level.ApMax != 34 || level.Ap != 32 || level.Accuracy != 13 {
		t.Fatalf("attack stats = min %d, max %d, display %d, accuracy %d", level.ApMin, level.ApMax, level.Ap, level.Accuracy)
	}
	if level.Evasion != 6 || level.DamageReduction != 3 || level.AddedEvasion != 16 || level.AddedDamageReduction != 26 {
		t.Fatalf("defense stats = evasion %d (+%d), DR %d (+%d)", level.Evasion, level.AddedEvasion, level.DamageReduction, level.AddedDamageReduction)
	}
	if level.CombatStats == nil || level.CombatStats.Magic == nil || level.CombatStats.Magic.AccuracyDice != "1D7+6" {
		t.Fatalf("magic combat lane = %#v", level.CombatStats)
	}
	if len(level.SpeciesAP) != 1 || level.SpeciesAP[0].Index != 1 || level.SpeciesAP[0].Value != 7 {
		t.Fatalf("species AP = %#v", level.SpeciesAP)
	}
	if level.EnhancementAids.Len() != 2 {
		t.Fatalf("enhancement aids = %d", level.EnhancementAids.Len())
	}
	if len(level.UnknownTail12) != 65 || level.UnknownTail12[0] != 1 || level.UnknownTail12[64] != 65 {
		t.Fatalf("unknown tail = %#v", level.UnknownTail12)
	}
	if got := level.UnknownFooter; len(got) != 6 || got[0] != 1 || got[5] != 6 {
		t.Fatalf("unknown footer = %#v", got)
	}

	apDirective := findEffect(t, level.Effects, "ALL_AP_INCRE")
	if len(apDirective.Args) != 0 {
		t.Fatalf("ALL_AP_INCRE raw args = %v", apDirective.Args)
	}
	if len(apDirective.CurveFields) != 1 || apDirective.CurveFields[0] != "ap" {
		t.Fatalf("ALL_AP_INCRE curve fields = %v", apDirective.CurveFields)
	}
	durabilityDirective := findEffect(t, level.Effects, "MAX_INDURANCE_INCRE")
	if durabilityDirective.Stat != "Max Durability Up" || len(durabilityDirective.CurveFields) != 1 || durabilityDirective.CurveFields[0] != "durability" {
		t.Fatalf("MAX_INDURANCE_INCRE = %#v", durabilityDirective)
	}
}

func TestParseFormulasKeepsArglessClientConstantRaw(t *testing.T) {
	groups, err := parseFormulas("ITEM_EFFECT();NU_ALL_REG_ADD()")
	if err != nil {
		t.Fatal(err)
	}
	effect := findEffect(t, groups, "NU_ALL_REG_ADD")
	if len(effect.Args) != 0 {
		t.Fatalf("raw args = %v", effect.Args)
	}
	if effect.Value != 10 || effect.Unit != "%" || effect.Op != "+" {
		t.Fatalf("formatted constant = %#v", effect)
	}
}

func TestParseFormulasRejectsUnknownSyntax(t *testing.T) {
	if _, err := parseFormulas("ITEM_EFFECT();BROKEN[4]"); err == nil {
		t.Fatal("expected unmatched DSL content error")
	}
	if _, err := parseFormulas("ITEM_EFFECT();HP_UP(not-a-number)"); err == nil {
		t.Fatal("expected invalid argument error")
	}
}

func findEffect(t *testing.T, groups []model.EffectGroup, function string) model.StatMod {
	t.Helper()
	for _, group := range groups {
		for _, stat := range group.Stats {
			if stat.EffectDsl != nil && stat.Func == function {
				return stat
			}
		}
	}
	t.Fatalf("effect %s not found", function)
	return model.StatMod{}
}

func enchantRecordFixture(baseID uint32) []byte {
	record := make([]byte, 0, 700)
	u8 := func(value byte) {
		record = append(record, value)
	}
	u16 := func(value uint16) {
		var raw [2]byte
		binary.LittleEndian.PutUint16(raw[:], value)
		record = append(record, raw[:]...)
	}
	u32 := func(value uint32) {
		var raw [4]byte
		binary.LittleEndian.PutUint32(raw[:], value)
		record = append(record, raw[:]...)
	}
	f32 := func(value float32) {
		u32(math.Float32bits(value))
	}
	text := func(value string) {
		units := utf16.Encode([]rune(value))
		var raw [8]byte
		binary.LittleEndian.PutUint64(raw[:], uint64(len(units)))
		record = append(record, raw[:]...)
		for _, unit := range units {
			u16(unit)
		}
	}
	tri := func(a, b, c float32) {
		f32(a)
		f32(b)
		f32(c)
	}

	u32(baseID)
	for value := uint32(1); value <= 5; value++ {
		u32(value)
	}
	u8(6)
	for value := uint32(7); value <= 13; value++ {
		u32(value)
	}
	u16(150)
	u16(14)
	u16(15)
	u8(16)
	u16(17)
	f32(50)
	for i := 0; i < 25; i++ {
		if i == 1 {
			f32(7)
		} else {
			f32(0)
		}
	}
	u8(2)
	tri(1, 2, 3)
	tri(0, 0, 0)
	tri(10, 20, 30)
	tri(14, 24, 34)
	tri(12, 22, 32)
	tri(1, 2, 3)
	tri(4, 5, 6)
	u32(18)
	text("custom enhancement text")
	text("ITEM_EFFECT();HP_UP(50);POTENTIAL_EFFECT();ALL_AP_INCRE();MAX_INDURANCE_INCRE()")
	u8(19)
	u8(20)
	u32(1_000_000)
	u32(700_000)
	u8(21)
	u8(22)
	u8(23)
	for i, dice := range []string{"1D3+4", "1D5+5", "1D7+6"} {
		text(dice)
		f32(float32(11 + i))
	}
	for i := 0; i < 3; i++ {
		f32(float32(4 + i))
		f32(float32(14 + i))
		f32(float32(1 + i))
		f32(float32(24 + i))
	}
	for i := 0; i < 12; i++ {
		u8(0xFF)
	}
	for i := 1; i <= 65; i++ {
		u8(byte(i))
	}
	u32(2)
	u32(45077)
	u32(767130)
	for i := 1; i <= 6; i++ {
		u8(byte(i))
	}
	return record
}
