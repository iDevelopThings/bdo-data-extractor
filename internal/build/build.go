// Package build assembles the unified item dataset (plus its sidecar JSON files)
// from the decoded client tables. The leaf parsers live in internal/tables; this
// package is the assembler that reads sources, joins them across tables, and
// writes the cached JSON for external consumption.
package build

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/idevelopthings/bdo-data-extractor/internal/config"
	"github.com/idevelopthings/bdo-data-extractor/internal/jsonio"
	"github.com/idevelopthings/bdo-data-extractor/internal/loc"
	"github.com/idevelopthings/bdo-data-extractor/internal/paz"
	"github.com/idevelopthings/bdo-data-extractor/internal/progress"
	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/src/model"
	"github.com/idevelopthings/bdo-data-extractor/src/utils"
)

// Builder holds the shared state every build stage needs, so stage methods read
// from fields instead of threading long argument lists. Stages run in order:
// each produces some state (items, regions, …) that later stages consume.
type Builder struct {
	src    *paz.Source
	gs     *loc.GameStrings
	dir    string // output directory (sidecar JSON lands here)
	lang   string
	region string // service region, e.g. na
	t0     time.Time

	// cross-stage state, set by an earlier stage and read by later ones
	items        map[uint32]*model.Item
	enhancements map[uint32]*model.Enhancement

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

	// writeDone delivers the result of the items/enhancements write, which runs in
	// the background (from buildItems) while the sidecar stages build. awaitWrite joins.
	writeDone chan writeResult
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

	b := &Builder{
		src:    src,
		lang:   *config.GlobalConfig.Lang,
		region: *config.GlobalConfig.Region,
		t0:     time.Now(),
	}
	Active = b

	b.logf(fmt.Sprintf("[INIT] Writing all files to: '%s'", outDir))

	for _, stage := range []struct {
		name string
		fn   func() error
	}{
		{name: "strings", fn: b.loadStrings},
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
		progress.Default().Phase(stage.name)
		if err := stage.fn(); err != nil {
			_ = b.awaitWrite() // join the background write before returning
			return err
		}
	}

	return b.awaitWrite()
}

func (b *Builder) logf(msg string) {
	progress.Default().Log(fmt.Sprintf("  [%5.1fs] %s", time.Since(b.t0).Seconds(), msg))
}

func (b *Builder) outFilePath(name string) (dir, filePath string) {
	dir = *config.GlobalConfig.Out
	filePath = filepath.Join(dir, name)
	return
}

// write emits one sidecar JSON file into the output directory and returns its path.
func (b *Builder) write(name string, v any) (string, error) {
	_, p := b.outFilePath(name)
	fn := filepath.Base(name)

	// err := utils.EnsureDirCreated(d)
	// if err != nil {
	// 	return "", err
	// }

	return fn, jsonio.WriteFile(p, v, *config.GlobalConfig.Pretty)
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
			"loc(%s): %d names, %d descriptions, %d market cats, %d buff names",
			b.lang, len(gs.Names), len(gs.Descs), len(gs.MarketCats), len(gs.BuffNames),
		),
	)

	return nil
}
