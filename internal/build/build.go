// Package build assembles the unified item dataset (plus its sidecar JSON files)
// from the decoded client tables. The leaf parsers live in internal/tables; this
// package is the assembler that reads sources, joins them across tables, and
// writes the cached JSON for external consumption.
package build

import (
	"fmt"
	"time"

	"github.com/idevelopthings/bdo-data-extractor/internal/config"
	"github.com/idevelopthings/bdo-data-extractor/internal/loc"
	"github.com/idevelopthings/bdo-data-extractor/internal/paz"
	"github.com/idevelopthings/bdo-data-extractor/internal/progress"
	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
	"github.com/idevelopthings/bdo-data-extractor/src/output"
	"github.com/idevelopthings/bdo-data-extractor/src/utils"
)

// Builder holds the shared state every build stage needs, so stage methods read
// from fields instead of threading long argument lists. Stages run in order:
// each produces some state (items, regions, …) that later stages consume.
type Builder struct {
	src    *paz.Source
	gs     *loc.GameStrings
	lang   string
	region string // service region, e.g. na
	t0     time.Time

	// cross-stage state, set by an earlier stage and read by later ones
	items           map[uint32]*model.Item
	enhancements    map[uint32]*model.Enhancement
	itemSets        []model.ItemSet
	questConditions map[uint32]tables.QuestConditionRow

	recipes []model.Recipe
	regions []model.WorldRegion
	npcs    []model.NPC
	caphras []model.CaphrasCategory

	// npcsDecoded memoizes the single npcsimply.bss decode shared by the recipe
	// stage (Korean-name lookup) and the NPC stage; see (*Builder).npcTable.
	npcsDecoded []model.NPC

	// nodesDecoded / waypointsDecoded memoize the worldmap node tables, shared by the
	// item stage (resolving <shop>/<node> acquisition to refs) and buildWorld, which
	// runs later; see (*Builder).explorationTable / waypointTable.
	nodesDecoded     []model.WorldNode
	waypointsDecoded map[uint32]tables.WorldWaypoint

	// characterFunction* memoizes characterfunction.dbss (+ offset), shared by
	// node-manager ownership and NPC item-service decoding in buildWorld / buildNpcs.
	characterFunctionOff  []byte
	characterFunctionData []byte

	// outputs owns staging and publication for the complete generated dataset.
	outputs *output.Transaction
}

// Active is the builder for the in-progress Run, exposed so other build-package
// code (and callers) can reach the shared state without it being threaded through.
var Active *Builder

// Release drops the finished Builder so the item/enhancement maps, localization
// tables, and PAZ index it holds become garbage. A long-running embedder should
// call it after a run to reclaim the memory; the CLI needn't bother (it exits).
func Release() {
	Active = nil
}

// Run reads every source table once, merges by item id, and writes outPath plus
// the sidecar JSON files. lang selects the localization (e.g. "en"); pretty
// indents the JSON output.
func Run() error {
	src, err := paz.OpenSource(*config.GlobalConfig.GameDir)
	if err != nil {
		return err
	}
	defer src.Close()

	outDir := *config.GlobalConfig.Out
	err = utils.EnsureDirCreated(outDir)
	if err != nil {
		return err
	}

	if err := src.Archive.AssertSafeOut(*config.GlobalConfig.Out); err != nil {
		return err
	}
	outputs, err := output.New(outDir, output.NewGoccyJSONDriver(*config.GlobalConfig.Pretty))
	if err != nil {
		return fmt.Errorf("initialize output transaction: %w", err)
	}

	b := &Builder{
		src:     src,
		outputs: outputs,
		lang:    *config.GlobalConfig.Lang,
		region:  *config.GlobalConfig.Region,
		t0:      time.Now(),
	}
	Active = b

	b.logf(fmt.Sprintf("writing to %s", outDir))

	runStage := func(name string, fn func() error) error {
		progress.Default().Phase(name)
		t0 := time.Now()
		if err := fn(); err != nil {
			return err
		}
		b.logStage(name, time.Since(t0))
		return nil
	}

	for _, stage := range []struct {
		name string
		fn   func() error
	}{
		{name: "strings", fn: b.loadStrings},
		{name: "character progression", fn: b.buildCharacterProgression},
		{name: "adventure journals", fn: b.buildAdventureJournals},
		{name: "life skill progression", fn: b.buildLifeSkillProgression},
		{name: "items", fn: b.buildItems},
		{name: "recipes", fn: b.buildRecipes},
		{name: "market categories", fn: b.buildMarketCategories},
		{name: "world", fn: b.buildWorld},
		{name: "npcs", fn: b.buildNpcs},
		{name: "zones", fn: b.buildZones},
		{name: "fishing", fn: b.buildFishing},
		{name: "knowledge", fn: b.buildKnowledge},
		{name: "mastery", fn: b.buildMastery},
		{name: "manufacture", fn: b.buildManufacture},
		{name: "caphras", fn: b.buildCaphras},
	} {
		if err := runStage(stage.name, stage.fn); err != nil {
			return err
		}
	}

	if err := runStage("write outputs", b.publishOutputs); err != nil {
		return err
	}
	b.logf(fmt.Sprintf("build finished in %s", utils.FormatDuration(time.Since(b.t0))))
	return nil
}

func (b *Builder) logf(msg string) {
	progress.Default().Log("  " + msg)
}

// logStage records one stage's wall time. Format is stable for scripts:
//
//	[done] <name>  <stageSeconds>s  (total <totalSeconds>s)
func (b *Builder) logStage(name string, d time.Duration) {
	progress.Default().Log(fmt.Sprintf(
		"[done] %s  %.3fs  (total %.3fs)",
		name, d.Seconds(), time.Since(b.t0).Seconds(),
	))
}

// logStep records a sub-stage timing inside a named stage. Format:
//
//	[step] <stage>/<name>  <seconds>s
func (b *Builder) logStep(stage, name string, d time.Duration) {
	progress.Default().Log(fmt.Sprintf("[step] %s/%s  %.3fs", stage, name, d.Seconds()))
}

// loadStrings decodes the localization tables (names, descriptions, market cats,
// buff names, and the various inline-name tables the later stages resolve against).
func (b *Builder) loadStrings() error {
	gs, err := loc.LoadGame(b.src.Archive.GameDir, b.lang)
	if err != nil {
		return err
	}
	b.gs = gs
	b.logf(
		fmt.Sprintf(
			"loc(%s): %d items, %d market cats, %d buff names",
			b.lang, len(gs.Items), len(gs.MarketCats), len(gs.BuffNames),
		),
	)

	return nil
}
