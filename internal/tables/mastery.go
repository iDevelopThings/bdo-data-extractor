package tables

import (
	"fmt"
	"math"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

// Life-skill mastery curve tables (gamecommondata/binary, PABR):
//   cookingstatdata.bss / alchemystatdata.bss / manufacturingstat.bss
// Each maps a mastery value to the proc/yield rates the client displays (via the
// ToClient_get{Cooking,Alchemy,Manufacturing}Stat* C++ APIs) and the engine uses
// to roll production results. Rates are stored as integers ×1e6.
//
// These are the client-side half of the production model: the per-recipe BASE
// output is not in the client (it's server-rolled; base is effectively 1), so the
// mastery curves are what determine the chance/size of extra ("proc") output.

// decodeRateCurve reads a PABR table of records [mastery f32, nRates × u32(/1e6)],
// count at offset 4, records from offset 8.
func decodeRateCurve(b []byte, nRates int) ([]model.MasteryBracket, error) {
	pabr, err := bss.OpenPABR(b)
	if err != nil {
		return nil, err
	}
	stride := (1 + nRates) * 4
	out := make([]model.MasteryBracket, 0, pabr.Rows)
	for i := 0; i < pabr.Rows; i++ {
		o := pabr.RecordsStart + i*stride
		if o+stride > pabr.StringTablePos {
			return nil, fmt.Errorf("mastery curve truncated at row %d", i)
		}
		rates := make([]float64, nRates)
		for r := 0; r < nRates; r++ {
			rates[r] = float64(bss.U32(b, o+4+r*4)) / 1e6
		}
		out = append(out, model.MasteryBracket{Mastery: int(math.Round(bss.F32(b, o))), Rates: rates})
	}

	return out, nil
}

// DecodeCookingMastery decodes cookingstatdata.bss (61 brackets, 5 rate columns).
func DecodeCookingMastery(b []byte) ([]model.MasteryBracket, error) {
	return decodeRateCurve(b, 5)
}

// DecodeAlchemyMastery decodes alchemystatdata.bss (61 brackets, 9 rate columns).
func DecodeAlchemyMastery(b []byte) ([]model.MasteryBracket, error) {
	return decodeRateCurve(b, 9)
}

// DecodeProcessingMastery decodes manufacturingstat.bss. The file holds 6 identical
// per-method sub-curves, each `[u32 count][count × {mastery f32, procRate u32/1e6,
// batch u32, 0}]`; the first sub-curve's count is at offset 8. They match, so we
// return the first. Note: offset 4 is 0 (not a row count) — this is PABR-magic only,
// not a full OpenPABR string-table table.
func DecodeProcessingMastery(b []byte) ([]model.ProcessingBracket, error) {
	if len(b) < 12 || string(b[:4]) != "PABR" {
		return nil, fmt.Errorf("manufacturingstat: not a PABR-framed table")
	}
	cnt := int(bss.U32(b, 8))
	if cnt <= 0 {
		return nil, fmt.Errorf("manufacturingstat: bad bracket count %d", cnt)
	}
	out := make([]model.ProcessingBracket, 0, cnt)
	for i := 0; i < cnt; i++ {
		o := 12 + i*16
		if o+16 > len(b) {
			return nil, fmt.Errorf("manufacturingstat: truncated at bracket %d", i)
		}
		out = append(
			out, model.ProcessingBracket{
				Mastery:  int(math.Round(bss.F32(b, o))),
				ProcRate: float64(bss.U32(b, o+4)) / 1e6,
				Batch:    int(bss.U32(b, o+8)),
			},
		)
	}

	return out, nil
}
