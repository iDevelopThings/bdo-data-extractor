package build

import (
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

// buildMastery decodes the life-skill mastery curve tables (cooking/alchemy/
// processing) into mastery.json — the client-side proc/yield rates keyed by mastery
// value, used to model production procs (base output is server-side, effectively 1).
// Skips if the tables are absent.
func (b *Builder) buildMastery() error {
	var mc model.MasteryCurves
	if d, err := b.src.Read("cookingstatdata.bss"); err == nil {
		mc.Cooking = tables.DecodeCookingMastery(d)
	}
	if d, err := b.src.Read("alchemystatdata.bss"); err == nil {
		mc.Alchemy = tables.DecodeAlchemyMastery(d)
	}
	if d, err := b.src.Read("manufacturingstat.bss"); err == nil {
		mc.Processing = tables.DecodeProcessingMastery(d)
	}
	if len(mc.Cooking)+len(mc.Alchemy)+len(mc.Processing) == 0 {
		return nil
	}
	p, err := b.write("mastery.json", mc)
	if err != nil {
		return err
	}
	b.logf(
		fmt.Sprintf(
			"mastery curves: cooking %d, alchemy %d, processing %d brackets -> %s",
			len(mc.Cooking), len(mc.Alchemy), len(mc.Processing), p,
		),
	)

	return nil
}

// buildManufacture decodes manufacture.bss into manufacture.json (recipe group,
// inputs, result item, action type, success rate — no yield count; yield is
// server-rolled). Skips if absent.
func (b *Builder) buildManufacture() error {
	d, err := b.src.Read("manufacture.bss")
	if err != nil {
		return nil
	}
	recs := tables.DecodeManufacture(d)
	if len(recs) == 0 {
		return nil
	}
	p, err := b.write("manufacture.json", recs)
	if err != nil {
		return err
	}
	b.logf(fmt.Sprintf("manufacture: %d recipes (group/inputs/type/success; output server-resolved) -> %s", len(recs), p))

	return nil
}
