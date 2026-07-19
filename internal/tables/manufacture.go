package tables

import (
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

// manufacture.bss — the unified manufacture/processing/alchemy/cooking recipe table
// (gamecommondata/binary, PABR). Variable-length records, each terminated by a
// single 0x28 separator byte. Carries the recipe group (ResultDropGroup), input
// materials+counts, the action type, and a success rate.
//
// NOT decoded here (deliberately): the OUTPUT. There is no output-count field (yield
// is rolled server-side — base output is effectively 1, mastery procs add more, see
// mastery.go), and the record's direct-result (결과물) slot is unreliable/legacy
// (often a copy of an input; verified to produce nonsense item↔output pairings), so
// it is dropped. The real output is the ResultDropGroup, which resolves server-side.
// Recipe INPUTS for items.json/recipes.json come from the per-item XMLs (keyed by the
// real output, richer coverage); this table is decoded for the success rate + type.

// DecodeManufacture parses every record. Record shape:
//
//	u32 group, u32 matCount(1..6), matCount×{u32 item, u32 count, u32 zero},
//	u32 결과물(legacy, dropped), u32 0, u32 0, u32 typeIndex(0..11), u32 successPercent(/1e6), u32 extra, u8 0x28
func DecodeManufacture(b []byte) ([]model.ManufactureRecipe, error) {
	pabr, err := bss.OpenPABR(b)
	if err != nil {
		return nil, err
	}
	stPos := pabr.StringTablePos
	u := func(p int) uint32 { return bss.U32(b, p) }
	isItem := func(v uint32) bool { return v >= 1 && v <= 1_000_000_000 }
	isCount := func(v uint32) bool { return v >= 1 && v <= 1_000_000 }

	// locate the first record (a small preamble precedes it)
	p := -1
	for q := 4; q+8 < stPos; q++ {
		if g, mc := u(q), u(q+4); g >= 1 && g <= 2_000_000 && mc >= 1 && mc <= 6 &&
			isItem(u(q+8)) && isCount(u(q+12)) && u(q+16) == 0 {
			p = q
			break
		}
	}
	if p < 0 {
		return nil, fmt.Errorf("manufacture: no recipe records found")
	}

	var out []model.ManufactureRecipe
	for p+8 < stPos {
		g, mc := u(p), u(p+4)
		if g < 1 || g > 2_000_000 || mc < 1 || mc > 6 {
			break
		}
		q := p + 8
		ins := make([]model.Ingredient, 0, mc)
		ok := true
		for k := 0; k < int(mc); k++ {
			it, cn := u(q), u(q+4)
			if !isItem(it) || !isCount(cn) || u(q+8) != 0 {
				ok = false
				break
			}
			ins = append(ins, model.Ingredient{Item: model.ItemRef(it), Count: int(cn)})
			q += 12
		}
		if !ok {
			break
		}
		out = append(
			out, model.ManufactureRecipe{
				Group:   g,
				Type:    manufactureActionName(u(q + 12)),
				Success: float64(u(q+16)) / 1e6,
				Inputs:  ins,
			},
		)
		p = q + 24 + 1 // 6-u32 tail + 1 separator byte
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("manufacture: decoded zero recipes")
	}
	return out, nil
}
