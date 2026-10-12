package tables

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

func TestDecodeBuffs(t *testing.T) {
	t.Parallel()

	record := testBuffRecord(t, uint16(321))
	data := make([]byte, 4, 4+len(record))
	binary.LittleEndian.PutUint32(data, 1)
	data = append(data, record...)

	offset := append([]byte("PABR"), 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(offset[4:], 1)
	offset = binary.LittleEndian.AppendUint16(offset, 321)
	offset = binary.LittleEndian.AppendUint32(offset, 4)
	offset = binary.LittleEndian.AppendUint32(offset, uint32(len(record)))

	buffs, err := DecodeBuffs(offset, data)
	if err != nil {
		t.Fatalf("DecodeBuffs() error = %v", err)
	}
	buff := buffs[321]
	if buff.Index != 321 || buff.NameKR != "시험" || buff.Module != 89 {
		t.Fatalf("DecodeBuffs() = %#v", buff)
	}
	if buff.DurationMs != 12_345 || buff.ApplyToGroup != "84" || buff.Icon != "icon.dds" {
		t.Fatalf("DecodeBuffs() did not preserve trailing fields: %#v", buff)
	}
	if buff.UnknownDuration[0] != 0xa1 || buff.StackingCategory != 26 || buff.UnknownTail26 != 0xb2 {
		t.Fatalf("DecodeBuffs() did not preserve unknown fields: %#v", buff)
	}
	control := Buff{Module: 58, StackingCategory: buff.StackingCategory}
	if category, ok := control.ClearsStackingCategory(); !ok || category != 2 {
		t.Fatalf("ClearsStackingCategory() = %d, %v, want 2, true", category, ok)
	}
}

// On client 3464 buffoffset.dbss keys buffs by u32 in 12-byte
// rows, each record leads with a u32 index, and skill records list u32 buffs.
func TestDecodeBuffsWithU32Keys(t *testing.T) {
	t.Parallel()

	const first, second = uint32(4_100_000_000), uint32(55_558)
	// A real skill key: small keys are ambiguous with offsets to the index column detection.
	const skillKey = uint32(3_735_879_681)
	firstRecord := testBuffRecord(t, first)
	secondRecord := testBuffRecord(t, second)
	data := binary.LittleEndian.AppendUint32(nil, 2)
	data = append(data, firstRecord...)
	data = append(data, secondRecord...)

	offset := append([]byte("PABR"), 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(offset[4:], 2)
	offset = binary.LittleEndian.AppendUint32(offset, first)
	offset = binary.LittleEndian.AppendUint32(offset, 4)
	offset = binary.LittleEndian.AppendUint32(offset, uint32(len(firstRecord)))
	offset = binary.LittleEndian.AppendUint32(offset, second)
	offset = binary.LittleEndian.AppendUint32(offset, uint32(4+len(firstRecord)))
	offset = binary.LittleEndian.AppendUint32(offset, uint32(len(secondRecord)))

	buffs, err := DecodeBuffs(offset, data)
	if err != nil {
		t.Fatalf("DecodeBuffs() error = %v", err)
	}
	if buffs[first].Index != first || buffs[second].Index != second || buffs[second].NameKR != "시험" {
		t.Fatalf("DecodeBuffs() = %#v", buffs)
	}

	skill := make([]byte, skillBuffList)
	binary.LittleEndian.PutUint32(skill[skillCooldown:], 5_000)
	skill = binary.LittleEndian.AppendUint32(skill, second)
	skill = binary.LittleEndian.AppendUint32(skill, first)
	skill = binary.LittleEndian.AppendUint32(skill, 0)
	skillOffset := append([]byte("PABR"), 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(skillOffset[4:], 1)
	skillOffset = binary.LittleEndian.AppendUint32(skillOffset, skillKey)
	skillOffset = binary.LittleEndian.AppendUint32(skillOffset, 4)
	skillOffset = binary.LittleEndian.AppendUint32(skillOffset, uint32(len(skill)))
	skillData := append(binary.LittleEndian.AppendUint32(nil, 1), skill...)

	effects, err := DecodeSkillEffects(skillOffset, skillData, buffs, true)
	if err != nil {
		t.Fatalf("DecodeSkillEffects() error = %v", err)
	}
	if got := effects[skillKey]; got.CooldownMs != 5_000 || len(got.Buffs) != 2 || got.Buffs[0] != second || got.Buffs[1] != first {
		t.Fatalf("DecodeSkillEffects() = %#v", got)
	}
}

// Older clients list u16 buff indices, and a record may end on the last one.
func TestDecodeSkillEffectsWithU16Buffs(t *testing.T) {
	t.Parallel()

	const buffIndex, skillKey = uint32(321), uint32(3_735_879_681)
	buffs := map[uint32]Buff{buffIndex: {Index: buffIndex}}
	skill := make([]byte, skillBuffList)
	skill = binary.LittleEndian.AppendUint16(skill, uint16(buffIndex))
	skillOffset := append([]byte("PABR"), 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(skillOffset[4:], 1)
	skillOffset = binary.LittleEndian.AppendUint32(skillOffset, skillKey)
	skillOffset = binary.LittleEndian.AppendUint32(skillOffset, 4)
	skillOffset = binary.LittleEndian.AppendUint32(skillOffset, uint32(len(skill)))
	skillData := append(binary.LittleEndian.AppendUint32(nil, 1), skill...)

	effects, err := DecodeSkillEffects(skillOffset, skillData, buffs, false)
	if err != nil {
		t.Fatalf("DecodeSkillEffects() error = %v", err)
	}
	if got := effects[skillKey]; len(got.Buffs) != 1 || got.Buffs[0] != buffIndex {
		t.Fatalf("DecodeSkillEffects() = %#v", got)
	}
}

func TestDecodeBuffsRejectsIndexKeyMismatch(t *testing.T) {
	t.Parallel()

	record := testBuffRecord(t, uint16(321))
	data := binary.LittleEndian.AppendUint32(nil, 1)
	data = append(data, record...)
	offset := binary.LittleEndian.AppendUint32(nil, 1)
	offset = binary.LittleEndian.AppendUint16(offset, 322)
	offset = binary.LittleEndian.AppendUint32(offset, 4)
	offset = binary.LittleEndian.AppendUint32(offset, uint32(len(record)))

	_, err := DecodeBuffs(offset, data)
	if err == nil || !strings.Contains(err.Error(), "record index is 321") {
		t.Fatalf("DecodeBuffs() error = %v, want key mismatch", err)
	}
}

func TestResolveBuffModules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		module  byte
		args    []int32
		stat    string
		statID  model.StatId
		value   float64
		unit    string
		instant bool
	}{
		{name: "energy", module: 79, args: []int32{10}, stat: "Energy Recovery", statID: model.StatIdEnergyRecovery, value: 10, instant: true},
		{name: "breath experience", module: 89, args: []int32{0, 100}, stat: "Breath EXP", statID: model.StatIdBreathExp, value: 100, instant: true},
		{name: "strength experience", module: 89, args: []int32{1, 125}, stat: "Strength EXP", statID: model.StatIdStrengthExp, value: 125, instant: true},
		{name: "health experience with rule flag", module: 89, args: []int32{2, 150, 1}, stat: "Health EXP", statID: model.StatIdHealthExp, value: 150, instant: true},
		{name: "worker stamina recovery", module: 63, args: []int32{10}, stat: "Worker Stamina Recovery", statID: model.StatIdWorkerStaminaRecovery, value: 10, instant: true},
		{name: "death penalty resistance", module: 90, args: []int32{50_000}, stat: "Death Penalty Resistance", statID: model.StatIdDeathPenaltyResistance, value: 5, unit: "%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			buff := Buff{Module: tt.module}
			for i, value := range tt.args {
				binary.LittleEndian.PutUint32(buff.EffectData[effSlotBase+i*effSlotStride:], uint32(value))
			}
			resolved, ok := ResolveBuffStat(buff)
			if !ok || resolved.Label != tt.stat || resolved.Op != "+" || resolved.Value != tt.value || resolved.Unit != tt.unit {
				t.Fatalf("ResolveBuffStat() = %#v, %v", resolved, ok)
			}
			if buff.IsInstant() != tt.instant {
				t.Fatalf("IsInstant() = %v, want %v", buff.IsInstant(), tt.instant)
			}
			if resolved.ID != tt.statID {
				t.Fatalf("ResolveBuffStat().ID = %q, want %q", resolved.ID, tt.statID)
			}
		})
	}
}

func TestResolveBuffStatUsesModuleSemantics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		module byte
		target int32
		want   model.StatId
	}{
		{module: 39, target: 3, want: model.StatIdHiddenAp},
		{module: 41, target: 3, want: model.StatIdEvasion},
		{module: 67, target: 0, want: model.StatIdMovementSpeedLevel},
		{module: 93, target: 4, want: model.StatIdCritDamage},
		{module: 149, target: int32(model.LifeSkillTypeCount), want: model.StatIdAllMastery},
	}
	for _, tt := range tests {
		buff := Buff{Module: tt.module}
		binary.LittleEndian.PutUint32(buff.EffectData[effSlotBase:], uint32(tt.target))
		binary.LittleEndian.PutUint32(buff.EffectData[effSlotBase+effSlotStride:], 1)
		if tt.module == 149 {
			binary.LittleEndian.PutUint32(buff.EffectData[effSlotBase+2*effSlotStride:], 1)
		}
		resolved, ok := ResolveBuffStat(buff)
		if !ok || resolved.ID != tt.want {
			t.Errorf("ResolveBuffStat(module %d) = %#v, %v, want ID %q", tt.module, resolved, ok, tt.want)
		}
	}
}

// testBuffRecord encodes a buff record; index is a uint16 or, for the wide
// layout, a uint32.
func testBuffRecord(t *testing.T, index any) []byte {
	t.Helper()

	var out bytes.Buffer
	write := func(value any) {
		t.Helper()
		if err := binary.Write(&out, binary.LittleEndian, value); err != nil {
			t.Fatalf("binary.Write() error = %v", err)
		}
	}
	writeUTF16 := func(value string) {
		units := utf16.Encode([]rune(value))
		write(int64(len(units)))
		for _, unit := range units {
			write(unit)
		}
	}
	writeUTF8 := func(value string) {
		write(int64(len(value)))
		_, _ = out.WriteString(value)
	}

	write(index)
	writeUTF16("시험")
	write(int16(-2))
	write(byte(3))
	write(byte(4))
	write(int16(55))
	write(int16(6))
	write(byte(89))
	write(byte(8))
	write(byte(1))
	write(byte(0))
	write([92]byte{7: 2, 15: 150})
	write(int32(12_345))
	unknownDuration := [25]byte{}
	unknownDuration[0] = 0xa1
	write(unknownDuration)
	writeUTF16("84")
	writeUTF8("icon.dds")
	write(byte(9))
	write(int32(10))
	writeUTF16("설명")
	unknownTail := [27]byte{}
	unknownTail[24] = 26
	unknownTail[26] = 0xb2
	write(unknownTail)
	return out.Bytes()
}
