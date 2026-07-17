package tables

// Buff/skill effect chain decoder.
//
// Consumables (food/elixirs, itemType "Skill") apply their effects via a skill:
//
//	itemenchant row  u32 @192            = skill key
//	skilloffset.dbss (key,off,size)      -> skill record in skill.dbss
//	skill record     u32 @95 = cooldown(ms), u16 list @99 = buff indices (till 0)
//	buff.dbss (schema.Buff)              -> Index -> {Name, DurationMs, ...}
//	loc table 5 (key1 0)                 -> buff Index -> English effect name
//
// buff.dbss is a flat schema (parsed via bss.ReadAll); skill.dbss has a
// variable-length buff list so it's read by offset.

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/internal/schema"
)

const (
	offSkillKey   = 192 // u32 in the itemenchant row -> skill key
	skillCooldown = 95  // u32 ms in the skill record
	skillBuffList = 99  // u16 buff indices in the skill record (until 0/invalid)
)

// Buff is the decoded buff.dbss data we use: its duration, internal name, and
// the structured effect (ModuleType = effect kind, EffectData = its parameters,
// Condition = the trigger for on-hit modules).
type Buff struct {
	DurationMs int
	NameKR     string
	Module     byte
	Condition  int16
	EffectData []byte

	OtherFields map[string]any `json:"-"`
}

// SkillEffect is one consumable skill: its use cooldown and the buffs it applies.
type SkillEffect struct {
	CooldownMs int
	Buffs      []uint16
}

// DecodeBuffs reads buff.dbss via the schema, keyed by buff Index.
func DecodeBuffs(data []byte) (map[uint16]Buff, error) {
	rows, err := bss.NewReader(data).ReadAll(schema.Buff)
	if err != nil {
		return nil, err
	}
	out := make(map[uint16]Buff, len(rows))
	for _, r := range rows {
		idx, ok := r["Index"].(uint16)
		if !ok {
			continue
		}
		if _, seen := out[idx]; seen {
			continue
		}
		dur, _ := r["DurationMs"].(int32)
		if dur < 0 || dur > 2_000_000_000 {
			dur = 0
		}
		name, _ := r["Name"].(string)
		mod, _ := r["ModuleType"].(byte)
		cond, _ := r["ConditionType"].(int16)
		eff, _ := r["EffectData"].([]byte)
		out[idx] = Buff{
			DurationMs:  int(dur),
			NameKR:      name,
			Module:      mod,
			Condition:   cond,
			EffectData:  eff,
			OtherFields: r,
		}
	}
	return out, nil
}

// DecodeSkillEffects maps skill key -> {cooldown, buff list}, from skilloffset.dbss
// + skill.dbss. buffs (from DecodeBuffs) terminates the variable-length buff list.
func DecodeSkillEffects(offsetRaw, data []byte, buffs map[uint16]Buff) (map[uint32]SkillEffect, error) {
	idx, err := bss.ParseOffsetIndex(offsetRaw, len(data))
	if err != nil {
		return nil, err
	}
	out := make(map[uint32]SkillEffect, len(idx))
	for _, e := range idx {
		rec, ok := e.Slice(data)
		if !ok || len(rec) < skillBuffList+2 {
			continue
		}
		if _, seen := out[e.Key]; seen {
			continue
		}
		var se SkillEffect
		if cd := int(bss.U32(rec, skillCooldown)); cd > 0 && cd <= 100_000_000 && cd%1000 == 0 {
			se.CooldownMs = cd
		}
		for j := skillBuffList; j+2 <= len(rec); j += 2 {
			b := bss.U16(rec, j)
			if b == 0 {
				break
			}
			if _, ok := buffs[b]; !ok {
				break
			}
			se.Buffs = append(se.Buffs, b)
			if len(se.Buffs) >= 40 {
				break
			}
		}
		out[e.Key] = se
	}
	return out, nil
}

// SkillKey reads the skill key (u32 @192) from an itemenchant row, or 0.
func SkillKey(rec []byte) uint32 {
	if len(rec) < offSkillKey+4 {
		return 0
	}
	return bss.U32(rec, offSkillKey)
}

// statRe splits an effect display name into "<stat> <op><value><unit>", e.g.
// "Fishing EXP +10%" -> ("Fishing EXP","+",10,"%"). Names that aren't a stat
// modifier (e.g. the "Satiated" debuff) don't match.
var statRe = regexp.MustCompile(`^(.+?)\s*([+\-])\s*([0-9]+(?:\.[0-9]+)?)\s*(%?)\s*$`)

// ParseStat turns a buff display name into typed stat fields. ok is false when
// the name has no "+N"/"-N" modifier (used to filter out non-stat debuffs).
func ParseStat(name string) (stat, op string, value float64, unit string, ok bool) {
	m := statRe.FindStringSubmatch(name)
	if m == nil {
		// fmt.Printf("WARNING: ParseStat failed to parse stat name %q\n", name)
		return "", "", 0, "", false
	}
	v, _ := strconv.ParseFloat(m[3], 64)
	return strings.TrimSpace(m[1]), m[2], v, m[4], true
}

// krFoodWrap strips the Korean food-buff wrapper and trailing duration so the
// inner "<stat> ±N[%]" can go through ParseStat, e.g.
// "음식 효과(건강 경험치 +100)" -> "건강 경험치 +100".
var krFoodWrap = regexp.MustCompile(`^음식 효과\((.*)\)$`)
var krDuration = regexp.MustCompile(`\s*\(?\d+(분|시간)\)?\s*$`)

// krStatEN translates the Korean stat names of hidden buffs to English. Extend
// as more hidden stats turn up; unmapped names fall through as raw Korean. The
// "획득"/"획득량" (gain/amount) variants normalize to the same English as the base.
var krStatEN = map[string]string{
	// combat
	"공격력":        "AP",
	"모든 공격력":     "All AP",
	"적중력":        "Accuracy",
	"모든 적중력":     "All Accuracy",
	"회피력":        "Evasion",
	"모든 회피력":     "All Evasion",
	"회피율":        "Evasion Rate",
	"공격 속도":      "Attack Speed",
	"시전 속도":      "Casting Speed",
	"피해 감소":      "Damage Reduction",
	"모든 피해 감소":   "All Damage Reduction",
	"몬스터 피해 감소":  "Damage Reduction vs Monsters",
	"몬스터 추가 공격력": "Extra AP vs Monsters",
	"몬스터 추가 피해":  "Extra Damage vs Monsters",
	"백어택 피해량":    "Back Attack Damage",
	"ALL 저항":     "All Resistance",
	// resources
	"생명력":        "HP",
	"최대 생명력":     "Max HP",
	"최대 지구력":     "Max Stamina",
	"최대 무게":      "Max Weight",
	"생명력 자연 회복량": "HP Recovery",
	"기운 자연 회복량":  "Energy Recovery",
	"정신력 자연 회복량": "MP/WP/SP Recovery",
	"탑승물 생명력":    "Mount HP",
	"말 생명력 회복":   "Horse HP Recovery",
	// Tiered horse-stamina buff "말 지구력 회복 5-1": the "5" stays in the name and
	// "-1" is mis-parsed as the value; the label is still right.
	"말 지구력 회복 5": "Horse Stamina Recovery",
	// life skill
	"생활 숙련도":       "Life Mastery",
	"가공 성공률":       "Processing Success Rate",
	"연금 시간":        "Alchemy Time",
	"요리 소요 시간":     "Cooking Time",
	"아이템 획득 확률":    "Item Drop Rate",
	"모든 채집물 획득 확률": "Gathering Drop Rate",
	"지식 획득 확률":     "Knowledge Drop Rate",
	"상위 지식 획득 확률":  "Higher Knowledge Drop Rate",
	// experience
	"건강 경험치":       "Health EXP",
	"생활 경험치":       "Life EXP",
	"생활 경험치 획득":    "Life EXP",
	"생활 경험치 획득량":   "Life EXP",
	"전투 경험치":       "Combat EXP",
	"전투 경험치 획득":    "Combat EXP",
	"전투 경험치 획득량":   "Combat EXP",
	"[이벤트] 전투 경험치": "[Event] Combat EXP",
	"기술 경험치":       "Skill EXP",
	"기술 경험치 획득":    "Skill EXP",
	"기술 경험치 획득량":   "Skill EXP",
	"탑승물 경험치":      "Mount EXP",
	"탑승물 경험치 획득량":  "Mount EXP",
	"힘 경험치":        "Strength EXP",
	"조련 경험치 획득":    "Taming EXP",
	"획득 생활 경험치":    "Life EXP",
	"지구력 경험치":      "Stamina EXP",
	"공헌도 경험치":      "Contribution EXP",
	"건강 경험치 획득량":   "Health EXP",
	// long tail
	"이동 속도":           "Movement Speed",
	"다운어택 피해량":        "Down Attack Damage",
	"에어어택 피해량":        "Air Attack Damage",
	"모든 특수 공격 추가 피해":  "All Special Attack Damage",
	"기운":              "Energy",
	"생명력 회복":          "HP Recovery",
	"정신력 회복":          "MP/WP/SP Recovery",
	"낚시 숙련도":          "Fishing Mastery",
	"모든 숙련도":          "All Mastery",
	"모든 채집물 획득 확률 증가": "Gathering Drop Rate",
	"희귀 어종을 낚을 확률":    "Rare Fish Catch Rate",
	"일꾼 행동력 추가":       "Worker Stamina",
	"사망 시 불이익 감소":     "Reduced Death Penalty",
	"사망 시 불이익 저항":     "Death Penalty Resistance",
	"길드 성향치":          "Guild Karma",
	"[식목일] 성향치":       "Karma",
	"[영웅] 즉시 회복 물약":   "Instant HP Recovery",
}

// ParseHiddenStat parses a hidden buff's Korean name into a stat modifier. The
// stat name is translated to English when known, else left as Korean (so it can
// be translated later). ok is false when there's no "±N" modifier.
func ParseHiddenStat(kr string) (stat, op string, value float64, unit string, ok bool) {
	s := kr
	if m := krFoodWrap.FindStringSubmatch(s); m != nil {
		s = m[1]
	}
	s = krDuration.ReplaceAllString(s, "")
	st, op, value, unit, ok := ParseStat(s)
	if !ok {
		return "", "", 0, "", false
	}
	st = strings.TrimSpace(st)
	if en, known := krStatEN[st]; known {
		st = en
	}
	return st, op, value, unit, true
}
