package build

import (
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
)

// buildMastery decodes the life-skill mastery curve tables (cooking/alchemy/
// processing) into mastery.json — the client-side proc/yield rates keyed by mastery
// value, used to model production procs (base output is server-side, effectively 1).
// Skips if the tables are absent; fails if a present table is corrupt.
func (b *Builder) buildMastery() error {
	var mc model.MasteryCurves

	d, ok, err := b.src.ReadIfExists("cookingstatdata.bss")
	if err != nil {
		return err
	}
	if ok {
		mc.Cooking, err = tables.DecodeCookingMastery(d)
		if err != nil {
			return fmt.Errorf("cookingstatdata: %w", err)
		}
	}

	d, ok, err = b.src.ReadIfExists("alchemystatdata.bss")
	if err != nil {
		return err
	}
	if ok {
		mc.Alchemy, err = tables.DecodeAlchemyMastery(d)
		if err != nil {
			return fmt.Errorf("alchemystatdata: %w", err)
		}
	}

	d, ok, err = b.src.ReadIfExists("manufacturingstat.bss")
	if err != nil {
		return err
	}
	if ok {
		mc.Processing, err = tables.DecodeProcessingMastery(d)
		if err != nil {
			return fmt.Errorf("manufacturingstat: %w", err)
		}
	}

	if len(mc.Cooking)+len(mc.Alchemy)+len(mc.Processing) == 0 {
		return nil
	}
	p, err := b.addJSON("mastery.json", mc)
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
// server-rolled). Skips if absent; fails if present but corrupt.
func (b *Builder) buildManufacture() error {
	d, ok, err := b.src.ReadIfExists("manufacture.bss")
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	recs, err := tables.DecodeManufacture(d)
	if err != nil {
		return err
	}
	p, err := b.addJSON("manufacture.json", recs)
	if err != nil {
		return err
	}
	b.logf(fmt.Sprintf("manufacture: %d recipes (group/inputs/type/success; output server-resolved) -> %s", len(recs), p))

	return nil
}
