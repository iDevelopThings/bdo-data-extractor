package main

import (
	"strings"
	"testing"
)

// build parses specs, wires the cross-spec registry, and resolves field types,
// mirroring the two-pass flow in main.
func build(t *testing.T, srcs ...string) []*parsed {
	t.Helper()
	all := make([]*parsed, 0, len(srcs))
	for _, src := range srcs {
		p, err := parse([]byte(src))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		all = append(all, p)
	}
	reg, err := buildRegistry(all)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	for _, p := range all {
		if err := resolveFields(p, reg); err != nil {
			t.Fatalf("resolve: %v", err)
		}
	}
	return all
}

func mustParse(t *testing.T, src string) *parsed {
	t.Helper()
	return build(t, src)[0]
}

const numericSpec = `
package: model
type: SlotName
underlying: byte
values:
  - name: Main
    wire: 0
    title: "Main Weapon"
  - name: Sub
    wire: 1
    title: "Sub-weapon"
  - name: MAX
    wire: 2
    sentinel: true
`

const stringSpec = `
package: model
type: Rarity
underlying: string
marshalText: true
values:
  - name: Common
    wire: common
    tier: 0
  - name: Rare
    wire: rare
    tier: 1
`

func TestRenderGoSentinelExcluded(t *testing.T) {
	p := mustParse(t, numericSpec)
	out, err := renderGo(p.spec, p.fields, p.entries, "slots.yml")
	if err != nil {
		t.Fatalf("renderGo: %v", err)
	}
	got := string(out)

	if !strings.Contains(got, "SlotNameMAX") {
		t.Error("sentinel const missing from generated source")
	}
	for _, marker := range []string{"slotNameByName", "slotNameValues", "slotNameInfos"} {
		if strings.Count(got, marker) == 0 {
			t.Errorf("expected %q in output", marker)
		}
	}
	if strings.Contains(got, "MAX:") || strings.Contains(got, `"max":`) {
		t.Error("sentinel leaked into a table")
	}
}

func TestRenderGoStringUnderlying(t *testing.T) {
	p := mustParse(t, stringSpec)
	out, err := renderGo(p.spec, p.fields, p.entries, "rarity.yml")
	if err != nil {
		t.Fatalf("renderGo: %v", err)
	}
	got := string(out)

	if !strings.Contains(got, `RarityCommon Rarity = "common"`) {
		t.Error("string wire value should be quoted in the const block")
	}
	if !strings.Contains(got, "func (v Rarity) Wire() string") {
		t.Error("Wire() should return the string underlying type")
	}
	if !strings.Contains(got, `fmt.Sprintf("Rarity(%q)", string(v))`) {
		t.Error("String() fallback should format a string underlying value")
	}
	if !strings.Contains(got, "func (v Rarity) MarshalText()") {
		t.Error("marshalText should emit MarshalText")
	}
}

func TestUnderlyingClassification(t *testing.T) {
	cases := []struct {
		underlying string
		wantString bool
		wantVerb   string
	}{
		{"int", false, "%d"},
		{"byte", false, "%d"},
		{"rune", false, "%d"},
		{"int8", false, "%d"},
		{"uint32", false, "%d"},
		{"int64", false, "%d"},
		{"uintptr", false, "%d"},
		{"float32", false, "%g"},
		{"float64", false, "%g"},
		{"string", true, "%q"},
	}
	for _, c := range cases {
		s := &spec{Underlying: c.underlying}
		if got := s.stringUnderlying(); got != c.wantString {
			t.Errorf("%s: stringUnderlying = %v, want %v", c.underlying, got, c.wantString)
		}
		if got := s.fmtVerb(); got != c.wantVerb {
			t.Errorf("%s: fmtVerb = %q, want %q", c.underlying, got, c.wantVerb)
		}
	}
}

func TestParseRejectsBadUnderlying(t *testing.T) {
	_, err := parse([]byte("package: p\ntype: T\nunderlying: notatype\nvalues:\n  - name: A\n"))
	if err == nil {
		t.Fatal("expected an error for an unknown underlying type")
	}
}

func TestRenderGoIntegerWidths(t *testing.T) {
	for _, u := range []string{"byte", "int8", "uint32", "rune"} {
		p := mustParse(t, "package: p\ntype: T\nunderlying: "+u+"\nvalues:\n  - name: A\n    wire: 1\n")
		out, err := renderGo(p.spec, p.fields, p.entries, "t.yml")
		if err != nil {
			t.Fatalf("%s: renderGo: %v", u, err)
		}
		got := string(out)
		if !strings.Contains(got, "type T "+u) {
			t.Errorf("%s: missing type declaration", u)
		}
		if !strings.Contains(got, "func (c TContainer) FromWire(w "+u+")") {
			t.Errorf("%s: FromWire should take the underlying type", u)
		}
	}
}

const statSpec = `
package: model
type: StatId
underlying: string
values:
  - name: fishingSpeed
  - name: critLevel
`

const funcSpec = `
package: model
type: EffectFuncStat
underlying: string
case: snake
fields:
  stat: StatId
  fanout: "[]StatId"
values:
  - name: FISHING_POINT
    stat: fishingSpeed
    fanout: [fishingSpeed, critLevel]
  - name: PLAIN
`

func TestRenderGoEnumRefs(t *testing.T) {
	all := build(t, statSpec, funcSpec)
	fn := all[1]
	out, err := renderGo(fn.spec, fn.fields, fn.entries, "effect_funcs.yml")
	if err != nil {
		t.Fatalf("renderGo: %v", err)
	}
	got := string(out)

	for _, want := range []string{
		"func (v EffectFuncStat) Stat() StatId",
		"func (v EffectFuncStat) Fanout() []StatId",
		"[]StatId{StatIdFishingSpeed, StatIdCritLevel}",
		"StatIdFishingSpeed,",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q\n---\n%s", want, got)
		}
	}
	// The PLAIN member sets neither ref; both should be omitted (Go zero-fills).
	if strings.Contains(got, "EffectFuncStatPLAIN: {\n\t\tEffectFuncStat: EffectFuncStatPLAIN,\n\t\tName: \"PLAIN\",\n\t\tStat:") {
		t.Error("absent ref should be omitted for PLAIN")
	}
}

func TestRenderGoEnumRefRejectsUnknownMember(t *testing.T) {
	const bad = `
package: model
type: EffectFuncStat
underlying: string
fields:
  stat: StatId
values:
  - name: X
    stat: notAStat
`
	all := build(t, statSpec, bad)
	_, err := renderGo(all[1].spec, all[1].fields, all[1].entries, "bad.yml")
	if err == nil {
		t.Fatal("expected an error referencing an unknown StatId member")
	}
	if !strings.Contains(err.Error(), "notAStat") {
		t.Errorf("error should name the bad member, got: %v", err)
	}
}

func TestRenderTS(t *testing.T) {
	p := mustParse(t, numericSpec)
	out, err := renderTS(p.spec, p.fields, p.entries, "slots.yml", "")
	if err != nil {
		t.Fatalf("renderTS: %v", err)
	}
	got := string(out)

	for _, want := range []string{
		"export const SlotNames = {",
		"export type SlotName = (typeof SlotNames)[keyof typeof SlotNames];",
		"export const SlotNameMAX = 2;",
		"export const SlotNameInfos: Record<SlotName, SlotNameInfo>",
		"title: string;",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("TS output missing %q\n---\n%s", want, got)
		}
	}
	if strings.Contains(got, "MAX:") {
		t.Error("sentinel should not appear in the TS values object")
	}
}

func TestRenderTSOmitsAbsentFields(t *testing.T) {
	const spec = `
package: p
type: Stat
underlying: string
values:
  - name: Hp
    label: "HP"
    unit: "%"
  - name: Recovery
    label: "Recovery"
`
	p := mustParse(t, spec)
	out, err := renderTS(p.spec, p.fields, p.entries, "stat.yml", "")
	if err != nil {
		t.Fatalf("renderTS: %v", err)
	}
	got := string(out)

	if !strings.Contains(got, "unit?: string;") {
		t.Errorf("unit should be an optional interface field\n---\n%s", got)
	}
	if strings.Contains(got, `unit: ""`) {
		t.Error("absent field should be omitted, not emitted as an empty string")
	}
	if !strings.Contains(got, `unit: "%"`) {
		t.Error("present field value should still be emitted")
	}
	if !strings.Contains(got, "label: string;") {
		t.Error("label should be a required interface field")
	}
}

func TestRenderTSSelfRef(t *testing.T) {
	const spec = `
package: model
type: StatId
underlying: string
tsOut: stats.ts
fields:
  fanout: "[]StatId"
values:
  - name: allAp
    fanout: [monsterAp, humanAp]
  - name: monsterAp
  - name: humanAp
`
	p := mustParse(t, spec)
	out, err := renderTS(p.spec, p.fields, p.entries, "stats.yml", "")
	if err != nil {
		t.Fatalf("renderTS: %v", err)
	}
	got := string(out)

	if !strings.Contains(got, "fanout?: StatId[];") {
		t.Errorf("fanout should be typed StatId[]\n---\n%s", got)
	}
	if !strings.Contains(got, "fanout: [StatIds.MonsterAp, StatIds.HumanAp]") {
		t.Errorf("self-ref slice should resolve to container accessors\n---\n%s", got)
	}
}

func TestIterators(t *testing.T) {
	const spec = `
package: p
type: Stat
underlying: string
tsOut: out.ts
iterators: [label, alias]
values:
  - name: Hp
    label: "HP"
    alias: hp
  - name: Mp
    label: "MP"
`
	p := mustParse(t, spec)

	goOut, err := renderGo(p.spec, p.fields, p.entries, "stat.yml")
	if err != nil {
		t.Fatalf("renderGo: %v", err)
	}
	got := string(goOut)
	if !strings.Contains(got, "func (c StatContainer) IterLabel() iter.Seq[string]") {
		t.Errorf("missing Go iterator method\n---\n%s", got)
	}
	if !strings.Contains(got, "yield(statInfos[v].Label)") {
		t.Error("Go iterator should yield the field value")
	}
	if !strings.Contains(got, "func (c StatContainer) IterAlias() iter.Seq[string]") {
		t.Error("missing second iterator")
	}

	tsOut, err := renderTS(p.spec, p.fields, p.entries, "stat.yml", "")
	if err != nil {
		t.Fatalf("renderTS: %v", err)
	}
	ts := string(tsOut)
	if !strings.Contains(ts, "export function* iterLabel(): Generator<string> {") {
		t.Errorf("missing TS iterator\n---\n%s", ts)
	}
	// alias is absent on Mp, so it's optional -> undefined in the element type.
	if !strings.Contains(ts, "export function* iterAlias(): Generator<string | undefined> {") {
		t.Error("optional-field iterator should include undefined")
	}
	if !strings.Contains(ts, "yield StatInfos[v].label;") {
		t.Error("TS iterator should yield the field value")
	}
}

func TestIteratorUnknownColumnErrors(t *testing.T) {
	const spec = `
package: p
type: Stat
underlying: string
iterators: [nope]
values:
  - name: Hp
    label: "HP"
`
	p := mustParse(t, spec)
	_, err := renderGo(p.spec, p.fields, p.entries, "stat.yml")
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("expected an error naming the unknown column, got: %v", err)
	}
}

func TestRenderTSCrossFileRef(t *testing.T) {
	const statSrc = `
package: model
type: StatId
underlying: string
tsOut: gen/stats.gen.ts
values:
  - name: fishingSpeed
  - name: critLevel
`
	const funcSrc = `
package: model
type: EffectFuncStat
underlying: string
case: snake
tsOut: gen/effects.gen.ts
fields:
  stat: StatId
values:
  - name: FISHING
    stat: fishingSpeed
`
	all := build(t, statSrc, funcSrc)
	// Simulate resolved absolute TS paths (same dir) so the relative import
	// specifier can be computed.
	statEntry := all[0]
	fnEntry := all[1]
	statEntry.tsOutAbs = "/proj/gen/stats.gen.ts"
	fnEntry.tsOutAbs = "/proj/gen/effects.gen.ts"
	// buildRegistry ran before we set tsOutAbs, so refresh the referenced entry.
	fnEntry.fields[0].Type.ref.tsOutAbs = statEntry.tsOutAbs

	out, err := renderTS(fnEntry.spec, fnEntry.fields, fnEntry.entries, "effect_funcs.yml", fnEntry.tsOutAbs)
	if err != nil {
		t.Fatalf("renderTS: %v", err)
	}
	got := string(out)

	if !strings.Contains(got, `import { StatIds, type StatId } from "./stats.gen";`) {
		t.Errorf("expected a cross-file import of the referenced enum\n---\n%s", got)
	}
	if !strings.Contains(got, "stat: StatId;") {
		t.Error("field should be typed with the imported enum type")
	}
	if !strings.Contains(got, "stat: StatIds.FishingSpeed") {
		t.Error("value should resolve to the imported container accessor")
	}
}

func TestRenderTSByNamePreservesCase(t *testing.T) {
	p := mustParse(t, stringSpec)
	out, err := renderTS(p.spec, p.fields, p.entries, "rarity.yml", "")
	if err != nil {
		t.Fatalf("renderTS: %v", err)
	}
	got := string(out)

	if !strings.Contains(got, `"Common": Raritys.Common`) {
		t.Errorf("ByName should preserve member-name casing\n---\n%s", got)
	}
	if strings.Contains(got, `"common": Raritys.Common`) {
		t.Error("ByName keys should not be lowercased")
	}
	if !strings.Contains(got, "export function parseRarity(name: string): Rarity | undefined") {
		t.Error("a case-insensitive parse helper should be generated")
	}
}
