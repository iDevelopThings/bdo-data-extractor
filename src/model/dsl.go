package model

import (
	"fmt"
	"log"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/src/utils"
)

// EffectDsl is one parsed DSL formula from an enchant record, e.g. HP_UP(110).
// Args are float64 because some formulas carry fractional values (e.g.
// ALCHEMY_REDUCE_TIME_DOWN(0.7)); whole numbers still marshal as `4`, not `4.0`.
type EffectDsl struct {
	Func string    `json:"func"`
	Args []float64 `json:"args,omitempty"`
}

type EffectGroup struct {
	Title  string    `json:"title"`
	Marker string    `json:"marker,omitempty"`
	Pieces int       `json:"pieces,omitempty"`
	Stats  []StatMod `json:"stats"`
}

// BuffStackingCategory is the client category used by broad buff-family rules.
// Known consumable values include food (1), elixir/draught (2), perfume (6),
// Cron-meal extras (10), and whale-tendon elixirs (21).
type BuffStackingCategory uint16

const (
	BuffStackingCategoryFood                BuffStackingCategory = 1
	BuffStackingCategoryElixir              BuffStackingCategory = 2
	BuffStackingCategoryPerfume             BuffStackingCategory = 6
	BuffStackingCategoryCronMealExtra       BuffStackingCategory = 10
	BuffStackingCategoryWhaleTendonElixir   BuffStackingCategory = 21
	BuffStackingCategoryDraughtResetControl BuffStackingCategory = 26
)

// BuffStackingCategories is a set of client-defined broad buff families.
type BuffStackingCategories []BuffStackingCategory

// Has reports whether the set contains category.
func (c BuffStackingCategories) Has(category ...BuffStackingCategory) bool {
	for _, candidate := range c {
		if slices.Contains(category, candidate) {
			return true
		}
	}
	return false
}

// Add inserts a nonzero category unless it is already present.
func (c *BuffStackingCategories) Add(category BuffStackingCategory) {
	if category == 0 || c.Has(category) {
		return
	}
	*c = append(*c, category)
}

// SortKey returns the consumable-family priority, with larger values sorting first.
func (c BuffStackingCategories) SortKey() int {
	switch {
	case c.Has(BuffStackingCategoryDraughtResetControl):
		return 6
	case c.Has(BuffStackingCategoryPerfume):
		return 5
	case c.Has(BuffStackingCategoryCronMealExtra):
		return 4
	case c.Has(BuffStackingCategoryFood):
		return 3
	case c.Has(BuffStackingCategoryElixir):
		return 2
	case c.Has(BuffStackingCategoryWhaleTendonElixir):
		return 1
	default:
		return 0
	}
}

// StatMod is one parsed effect line, e.g. "Fishing EXP +10%" ->
// {Stat:"Fishing EXP", Op:"+", Value:10, Unit:"%"}.
type StatMod struct {
	*EffectDsl

	Stat string `json:"stat,omitempty"`
	// StatID is the canonical accumulator key for a single-stat effect.
	StatID StatId `json:"statId,omitempty"`
	// StatIDs contains canonical keys for effects that apply to several peers.
	StatIDs     []StatId `json:"statIds,omitempty"`
	Op          string   `json:"op,omitempty"`
	Value       float64  `json:"value,omitempty"`
	Unit        string   `json:"unit,omitempty"`
	CurveFields []string `json:"curveFields,omitempty"`
	Buff        uint32   `json:"buff,omitempty"` // source buff Index (traceability)
	// BuffModule identifies the buff.dbss EffectData layout.
	BuffModule uint8 `json:"buffModule,omitempty"`
	// BuffGroup identifies mutually replacing variants of the same effect.
	BuffGroup int16 `json:"buffGroup,omitempty"`
	// BuffCategory identifies the broader family used by reset/stacking rules.
	BuffCategory BuffStackingCategory `json:"buffCategory,omitempty"`
	// DurationMs is this modifier's duration, independent of sibling effects.
	DurationMs int `json:"durationMs,omitempty"`
	// Instant identifies an immediate effect such as Energy or Health EXP gain.
	Instant bool   `json:"instant,omitempty"`
	Negate  bool   `json:"negate,omitempty"` // true if less is better, ie weight
	Note    string `json:"note,omitempty"`   // optional consumer-facing note (e.g. "hidden stat")
	// DerivedFrom identifies the raw DSL marker that implies a canonical effect.
	DerivedFrom string `json:"derivedFrom,omitempty"`
}

var statIDsByLabel = func() map[string]StatId {
	ids := make(map[string]StatId, len(StatIds.Infos()))
	for _, info := range StatIds.Infos() {
		ids[info.Label] = info.StatId
	}
	return ids
}()

// StatIDFromLabel resolves a stable display label to its canonical stat ID.
func StatIDFromLabel(label string) (StatId, bool) {
	id, ok := statIDsByLabel[label]
	return id, ok
}

func applyEffectStatIDs(mod StatMod, fn string) StatMod {
	info, ok := EffectFuncStat(fn).TryGetInfo()
	if !ok {
		return mod
	}
	mod.StatID = info.Stat
	mod.StatIDs = info.Stats
	return mod
}

// setPieceCount pulls the piece-count tier out of a set-effect marker func —
// NO_2_SET_EFFECT, ANCIENT_NO_2_SET_EFFECT, BLACKSTAR_NO_3_SET_EFFECT_1,
// NO_6_WEAR_EFFECT, NO_8_CASH_UP all encode it as NO_<n>_ — or 0 if none.
var setPieceCountRe = regexp.MustCompile(`NO_(\d+)_`)

func setPieceCount(fn string) int {
	if m := setPieceCountRe.FindStringSubmatch(fn); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	return 0
}

// sectionMarkerFor resolves the display section a marker func starts, if any.
// SET_EFFECT/WEAR_EFFECT/CASH_UP match by substring (not just the exact keys
// above) since boss and costume gear mint per-item variants —
// ANCIENT_NO_2_SET_EFFECT, BLACKSTAR_NO_3_SET_EFFECT_1, NO_6_WEAR_EFFECT,
// NO_8_CASH_UP, ... — that all belong under the "Set Effect" family. The piece
// count (2/4/6/8) is kept in the title so an item's tiers stay distinct sections
// instead of collapsing into one repeated "Set Effect".
func sectionMarkerFor(fn string) (string, bool) {
	title := ""
	if info, ok := EffectFuncStat(fn).TryGetInfo(); ok {
		title = info.SectionMarker
	}
	exact := title != ""
	if !exact && (strings.Contains(fn, "SET_EFFECT") || strings.Contains(fn, "WEAR_EFFECT") || strings.Contains(fn, "CASH_UP")) {
		title, exact = "Set Effect", true
	}
	if !exact {
		return "", false
	}
	if title == "Set Effect" {
		if n := setPieceCount(fn); n > 0 {
			return fmt.Sprintf("Set Effect (%d-piece)", n), true
		}
	}
	return title, true
}

// EffectFuncInfo is a DSL func's display info; only what formatting needs
// (the gear builder's stat aggregation keeps its own stat/apStat mapping —
// see frontend/src/lib/effect-dsl.ts).
type EffectFuncInfo struct {
	label  string
	unit   string
	negate bool // "..._DOWN" funcs carry positive args but reduce the stat (time costs)
}

// effectRule returns a DSL func's rule metadata and whether it carries one:
// curve fields whose value lives on the enhancement curve, a fixed client
// value, or bundled derived effects. Explicit arguments always take precedence
// over a rule. Sourced from the generated EffectFuncStat enum (see
// enums/effect_funcs.yml).
func effectRule(fn string) (EffectFuncStatInfo, bool) {
	info, ok := EffectFuncStat(fn).TryGetInfo()
	if !ok {
		return info, false
	}
	has := len(info.CurveFields) != 0 || info.FixedValue != 0 || len(info.Derived) != 0
	return info, has
}

func FormatEffectFunctions(effects []EffectDsl, hasMarkers bool, groupTitle string) []EffectGroup {
	groups := make([]EffectGroup, 0)

	var current *EffectGroup = nil
	if !hasMarkers {
		current = &EffectGroup{Title: groupTitle}
	}

	for _, e := range effects {
		if title, ok := sectionMarkerFor(e.Func); ok {
			if current != nil {
				groups = append(groups, *current)
			}
			current = &EffectGroup{
				Title:  title,
				Marker: e.Func,
				Pieces: setPieceCount(e.Func),
			}
			continue
		}

		if current == nil {
			current = &EffectGroup{Title: "Effects"}
		}
		if current != nil {
			mods, _ := EffectFuncToStatMods(e)
			for _, mod := range mods {
				if current.Title == "Enhancement Effect" {
					if mod.Value == 0 && !strings.HasSuffix(mod.Stat, " Up") {
						mod.Stat = mod.Stat + " Up"
					}
				}
				current.Stats = append(current.Stats, mod)
			}
		} else {
			log.Printf("WARNING: effect %q is not in a section, skipping", e.Func)
		}
	}

	if current != nil {
		groups = append(groups, *current)
	}

	return groups
}

// EffectFuncToStatMods resolves every stat represented by one DSL function.
func EffectFuncToStatMods(e EffectDsl) ([]StatMod, bool) {
	primary, ok := EffectFuncToStatMod(e)
	if !ok {
		return nil, false
	}
	mods := []StatMod{primary}
	info, hasRule := effectRule(e.Func)
	if !hasRule || len(e.Args) != 0 {
		return mods, true
	}
	for _, effect := range info.Derived {
		mod, resolved := EffectFuncToStatMod(effect)
		if !resolved {
			continue
		}
		mod.DerivedFrom = e.Func
		mod.Note = "Fixed client value bundled with " + e.Func
		mods = append(mods, mod)
	}
	return mods, true
}

func EffectFuncToStatMod(e EffectDsl) (StatMod, bool) {
	hasArg := len(e.Args) > 0
	arg := 0.0
	if hasArg {
		arg = e.Args[0]
	}

	rule, hasRule := effectRule(e.Func)
	if hasRule && len(rule.CurveFields) != 0 && !hasArg {
		label := e.Func
		if named, found := resolveNamedEffect(e.Func); found {
			label = named
		} else if info, found := ResolveEffectFunc(e.Func); found {
			label = info.label
		}
		return applyEffectStatIDs(StatMod{
			EffectDsl:   &e,
			Stat:        label,
			CurveFields: rule.CurveFields,
			Note:        "Value is stored on the containing enhancement curve",
		}, e.Func), true
	}

	if hasRule && rule.FixedValue != 0 && !hasArg {
		info, found := ResolveEffectFunc(e.Func)
		if !found {
			return StatMod{}, false
		}
		return applyEffectStatIDs(StatMod{
			EffectDsl: &e,
			Stat:      info.label,
			Op:        "+",
			Value:     rule.FixedValue,
			Unit:      info.unit,
			Note:      "Fixed client value; the DSL carries no argument",
		}, e.Func), true
	}

	if label, ok := resolveNamedEffect(e.Func); ok {
		mod := StatMod{
			EffectDsl: &e,
			Stat:      label,
			Value:     arg,
		}
		if hasArg {
			mod.Op = "+"
		} else {
			mod.Note = "Named effect; the DSL carries no numeric value"
		}
		return applyEffectStatIDs(mod, e.Func), true
	}

	if info, ok := ResolveEffectFunc(e.Func); ok {
		sign := "+"
		if info.negate {
			sign = "-"
		}
		m := StatMod{
			EffectDsl: &e,
			Stat:      info.label,
			Op:        sign,
			Negate:    info.negate,
			Value:     arg,
			Unit:      info.unit,
		}

		if !hasArg {
			m.Note = "Unknown (likely hard-coded stat value in client)"
			m.Op = "X"
		}

		return applyEffectStatIDs(m, e.Func), true
	}

	m := StatMod{
		EffectDsl: &EffectDsl{
			Func: e.Func,
			Args: e.Args,
		},
		Stat:  utils.HumanizeString(e.Func),
		Op:    "+",
		Value: arg,
	}
	if !hasArg {
		// fmt.Printf("WARNING: effect func %q is unmapped and has no arg\n", e.Func)
		m.Op = "X"
		m.Note = "Unmapped effect func with no arg"
	}

	return applyEffectStatIDs(m, e.Func), true
}

func ResolveEffectFunc(fn string) (EffectFuncInfo, bool) {
	if info, ok := EffectFuncStat(fn).TryGetInfo(); ok && info.Label != "" {
		return EffectFuncInfo{label: info.Label, unit: info.Unit, negate: info.Negate}, true
	}
	return EffectFuncInfo{}, false
}

func resolveNamedEffect(fn string) (string, bool) {
	if info, ok := EffectFuncStat(fn).TryGetInfo(); ok && info.Valueless && info.Label != "" {
		return info.Label, true
	}
	return "", false
}
