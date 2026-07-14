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

func GetLogReporter() Reporter {
	return progress.Log{}
}

// SetReporter installs r as the process-wide progress sink. Runs are not
// concurrency-safe (the sink and config.GlobalConfig are globals) — serialize them.
func SetReporter(r Reporter) {
	progress.Set(r)
}

// Configure populates the extractor's global config without running anything, for
// embedders that want to drive individual steps rather than RunAll.
func Configure(gameDir, dataDir, lang, region string, pretty bool) {
	config.Set(gameDir, dataDir, lang, region, pretty)
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

// AvailableRegions lists the region-data suffixes shipped by a game install.
// Archives do not distinguish resource baselines from service-region overrides.
func AvailableRegions(gameDir string) ([]string, error) {
	ix, err := paz.LoadMeta(gameDir)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	regions := make([]string, 0)
	for _, name := range ix.FileNames {
		base := filepath.Base(name)
		if !strings.HasPrefix(base, "regionclientdata_") || !strings.HasSuffix(base, "_.xml") {
			continue
		}
		r := strings.TrimSuffix(strings.TrimPrefix(base, "regionclientdata_"), "_.xml")
		if r != "" && !seen[r] {
			seen[r] = true
			regions = append(regions, r)
		}
	}
	sort.Strings(regions)

	return regions, nil
}

// Options configures a full extraction run. DataDir receives items.json plus the
// sidecar JSON, icons/, knowledge_icons/ and regionmaps/ subdirectories.
type Options struct {
	GameDir string
	DataDir string
	Lang    string // defaults to "en"
	// Region selects the game service region, such as "na". Region-aware extraction
	// stages apply the corresponding service data over their resource baseline.
	Region string
	Pretty bool
	// AppVersion is the embedding app's version, recorded in the data dir's
	// manifest so a stale-data check (NeedsExtraction) can tell when the app that
	// produced the data has since updated. Empty for the CLI.
	AppVersion string
}

// RunAll runs the pipeline the embedder needs (build → icons), reporting two
// top-level steps through the current sink.
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
	config.Set(o.GameDir, o.DataDir, o.Lang, o.Region, o.Pretty)

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
	const steps = 3

	rep.Step(1, steps, "build")
	if err := build.Run(); err != nil {
		return err
	}
	build.Release() // free the Builder's item/enhancement/loc maps before later steps

	// Icons and the world map are derived art: they depend only on the game files
	// plus their own codec version, not the app version, so an app-only update reuses
	// them. A game patch or a codec bump moves the key and forces a rebuild.
	rep.Step(2, steps, "icons")
	if err := ensureAsset(iconAsset, o, rep, Icons); err != nil {
		return err
	}

	rep.Step(3, steps, "world map")
	if err := ensureAsset(worldMapAsset, o, rep, WorldMap); err != nil {
		return err
	}

	if err := writeManifest(o.DataDir, o.GameDir, o.AppVersion, o.Lang, o.Region); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	return nil
}

// ensureAsset runs produce unless dataDir already holds a's output for this game +
// codec version, stamping the provenance afterwards so the next run can skip it.
func ensureAsset(a asset, o Options, rep Reporter, produce func() error) error {
	if a.fresh(o.DataDir, o.GameDir) {
		rep.Log(fmt.Sprintf("%s up to date for this game version — skipping", a.name))
		return nil
	}
	if err := produce(); err != nil {
		return err
	}
	if err := a.stamp(o.DataDir, o.GameDir); err != nil {
		return fmt.Errorf("write %s provenance: %w", a.name, err)
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
