// Package pipeline is the public entry point to the data-extraction pipeline: it
// lets an embedding app (not just the CLI) run extraction in-process by populating
// the extractor's single source of truth (config.GlobalConfig) and driving the
// same build/loc/icon steps the CLI runs. Progress flows through the settable sink
// in internal/progress — SetReporter swaps in an embedder's own (default: stdout).
package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/build"
	"github.com/idevelopthings/bdo-data-extractor/internal/config"
	"github.com/idevelopthings/bdo-data-extractor/internal/paz"
	"github.com/idevelopthings/bdo-data-extractor/internal/progress"
)

// DefaultGameDir is the default Steam install path, re-exported so embedders can
// prefill a directory picker without importing an internal package.
const DefaultGameDir = paz.DefaultGameDir

// Reporter is the progress sink. It is re-exported so embedders can implement it
// without importing internal/progress (which they cannot, being a separate module).
type Reporter = progress.Reporter

// SetReporter installs r as the process-wide progress sink. Runs are not
// concurrency-safe (the sink and config.GlobalConfig are globals) — serialize them.
func SetReporter(r Reporter) {
	progress.Set(r)
}

// Configure populates the extractor's global config without running anything, for
// embedders that want to drive individual steps rather than RunAll.
func Configure(gameDir, dataDir, lang string, pretty bool) {
	config.Set(gameDir, dataDir, lang, pretty)
}

// Meta is a lightweight summary of a game install's PAZ index, for confirming a
// user picked a valid directory before extracting.
type Meta struct {
	Version   uint32 `json:"version"`
	PazCount  uint32 `json:"pazCount"`
	Files     int    `json:"files"`
	Folders   int    `json:"folders"`
	FileNames int    `json:"fileNames"`
}

// ValidateGameDir confirms dir is a readable BDO install by parsing its PAZ meta,
// returning a summary. A non-nil error means the directory is not a valid install.
func ValidateGameDir(dir string) (Meta, error) {
	ix, err := paz.LoadMeta(dir)
	if err != nil {
		return Meta{}, err
	}

	return Meta{
		Version:   ix.Version,
		PazCount:  ix.PazCount,
		Files:     len(ix.Files),
		Folders:   len(ix.FolderNames),
		FileNames: len(ix.FileNames),
	}, nil
}

// AvailableLanguages lists the localization languages present in a game install,
// discovered from ads/languagedata_<lang>.loc (the extractor has no fixed list).
func AvailableLanguages(gameDir string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(gameDir, "ads", "languagedata_*.loc"))
	if err != nil {
		return nil, err
	}

	langs := make([]string, 0, len(matches))
	for _, m := range matches {
		lang := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(m), "languagedata_"), ".loc")
		if lang != "" {
			langs = append(langs, lang)
		}
	}
	sort.Strings(langs)

	return langs, nil
}

// Options configures a full extraction run. DataDir receives items.json plus the
// sidecar JSON, icons/, knowledge_icons/ and regionmaps/ subdirectories.
type Options struct {
	GameDir string
	DataDir string
	Lang    string // defaults to "en"
	Pretty  bool
	// AppVersion is the embedding app's version, recorded in the data dir's
	// manifest so a stale-data check (NeedsExtraction) can tell when the app that
	// produced the data has since updated. Empty for the CLI.
	AppVersion string
}

// RunAll runs the complete pipeline (build → localization → icons → knowledge
// icons → region maps), reporting five top-level steps through the current sink.
func RunAll(o Options) error {
	if o.Lang == "" {
		o.Lang = "en"
	}
	if err := assertOutsideGameDir(o.DataDir, o.GameDir); err != nil {
		return err
	}
	if err := os.MkdirAll(o.DataDir, 0o755); err != nil {
		return err
	}

	// Populate the single source of truth; every step below reads it.
	config.Set(o.GameDir, o.DataDir, o.Lang, o.Pretty)

	// Extraction allocates hundreds of MB (item/enhancement maps, the loc tables,
	// the PAZ index). In a long-running embedder those would stay resident via the
	// package globals, so release them and hand the heap back to the OS on the way
	// out — the CLI just exits, but an app (the viewer) gets its memory back.
	defer func() {
		build.Release()
		runtime.GC()
		debug.FreeOSMemory()
	}()

	rep := progress.Default()
	const steps = 5

	rep.Step(1, steps, "build")
	if err := build.Run(filepath.Join(o.DataDir, "items.json")); err != nil {
		return err
	}
	build.Release() // free the Builder's item/enhancement/loc maps before later steps
	rep.Step(2, steps, "localization")
	if err := Loc(); err != nil {
		return err
	}
	rep.Step(3, steps, "icons")
	if err := Icons(); err != nil {
		return err
	}
	rep.Step(4, steps, "knowledge icons")
	if err := KnowledgeIcons(); err != nil {
		return err
	}
	rep.Step(5, steps, "region maps")
	if err := RegionMaps(); err != nil {
		return err
	}

	if err := writeManifest(o.DataDir, o.GameDir, o.AppVersion, o.Lang); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	return nil
}

// assertOutsideGameDir refuses an output dir inside the read-only game install.
// build/icons/knowledge guard themselves via Archive.AssertSafeOut, but loc and
// regionmaps do not, so RunAll checks up front for the whole run.
func assertOutsideGameDir(out, gameDir string) error {
	outAbs, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	gameAbs, err := filepath.Abs(gameDir)
	if err != nil {
		return err
	}
	if strings.HasPrefix(strings.ToLower(outAbs), strings.ToLower(gameAbs)) {
		return fmt.Errorf("output dir %q is inside the game dir %q", out, gameDir)
	}

	return nil
}
