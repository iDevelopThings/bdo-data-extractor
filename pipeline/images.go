package pipeline

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/deepteams/webp"
	"github.com/dgravesa/go-parallel/parallel"

	"github.com/idevelopthings/bdo-data-extractor/internal/config"
	"github.com/idevelopthings/bdo-data-extractor/internal/jsonio"
	"github.com/idevelopthings/bdo-data-extractor/internal/paz"
	"github.com/idevelopthings/bdo-data-extractor/internal/progress"
	"github.com/idevelopthings/bdo-data-extractor/internal/tex"
)

// imageSource is one family of extractable images (item icons, region maps, …).
// Prepare reads whatever drives the source — a produced JSON, a decoded table, or
// nothing for a pure archive-path scan — and captures the paz.Source for Convert.
// runImages then does ONE pass over the archive index, dispatches each path to
// every source via Wants, and decodes + writes the matches in parallel.
type imageSource interface {
	// Name is the source's label for progress and logging.
	Name() string
	// Dir is the source's output subdirectory under the data dir (e.g. "icons").
	Dir() string
	// Prepare reads the source's driving data and captures src for Convert. It runs
	// once, before the index pass.
	Prepare(src *paz.Source, dataDir string) error
	// Wants reports whether the archive path belongs to this source. It is called
	// from the single-threaded index pass, so it may record path-derived state.
	Wants(path string) bool
	// Convert decodes one matched archive entry into one or more output files, each
	// relative to Dir. It runs concurrently, so it must not mutate shared state.
	Convert(path string, f paz.PazFile) ([]output, error)
	// Redirects returns request-path → served-file aliases for the viewer's asset
	// layer, both relative to the data dir. matched is the archive paths this source
	// claimed (for scan sources that derive aliases from the path); JSON-driven
	// sources build theirs in Prepare and ignore it. Nil when the source needs none.
	Redirects(matched []string) map[string]string
	// Rebuild reports whether Dir should be wiped before writing — for sources whose
	// shared-file naming would otherwise leave stale files from a prior run.
	Rebuild() bool
}

// output is one file a source produces: its path relative to the source's Dir and
// the encoded bytes (PNG, JSON, …). The runner writes it and tracks progress.
type output struct {
	Rel  string
	Data []byte
}

// assetRedirectFile is the merged alias table runImages writes at the data-dir
// root. Its keys/targets are data-dir-relative, so the viewer applies them to any
// asset request regardless of subdirectory.
const assetRedirectFile = "asset_redirects.json"

// Icons decodes every item, knowledge, territory and zone-category icon into the
// data dir, sharing decoded files across the ids that reference them and writing the
// asset alias table. Run build first (it reads items.json / knowledge.json / etc.).
func Icons() error {
	return runImages(itemIcons(), knowledgeIcons(), territoryIcons(), zoneCategoryIcons())
}

// Maps decodes the region-mask maps and builds the world-map tile pyramid into the data
// dir. The pyramid runs standalone (see WorldMap), not through runImages.
func Maps() error {
	// Kept for debugging
	// return runImages(worldMiniMaps())

	// Kept for now, but not used, just incase we want to add it back.
	// if err := runImages(regionMaps()); err != nil {
	// 	return err
	// }

	return WorldMap()
}

// KnowledgeIcons decodes just the knowledge-card icons, for rerunning that one set.
func KnowledgeIcons() error {
	return runImages(knowledgeIcons())
}

// RegionMaps decodes just the region-mask maps.
func RegionMaps() error {
	return runImages(regionMaps())
}

// runImages runs the given image sources against one game install: it opens the
// archive once, prepares each source, does a single index pass dispatching every
// path to the sources that want it, then decodes and writes all matches in
// parallel. Redirect tables from every source are merged into asset_redirects.json.
func runImages(sources ...imageSource) error {
	gameDir := *config.GlobalConfig.GameDir
	dataDir := *config.GlobalConfig.Out

	src, err := paz.OpenSource(gameDir)
	if err != nil {
		return err
	}
	defer src.Close()

	rep := progress.Default()
	// Set up every source's output dir in phases so a Rebuild wipe of a parent dir
	// (item icons wipe "icons") can't clobber a child dir (territory "icons/territories")
	// that another source just created: wipe all, then create all, then prepare all.
	for _, s := range sources {
		if err := src.Archive.AssertSafeOut(filepath.Join(dataDir, s.Dir())); err != nil {
			return err
		}
	}
	for _, s := range sources {
		if s.Rebuild() {
			// A source that names files by shared content (item icons) would otherwise
			// leave old per-id files, or files that changed between patches, lingering.
			if err := os.RemoveAll(filepath.Join(dataDir, s.Dir())); err != nil {
				return err
			}
		}
	}
	for _, s := range sources {
		if err := os.MkdirAll(filepath.Join(dataDir, s.Dir()), 0o755); err != nil {
			return err
		}
	}
	for _, s := range sources {
		if err := s.Prepare(src, dataDir); err != nil {
			return fmt.Errorf("%s: prepare: %w", s.Name(), err)
		}
	}

	// One pass over the whole index: each path goes to every source that claims it.
	type job struct {
		src  imageSource
		path string
		file paz.PazFile
	}
	var jobs []job
	matched := make([][]string, len(sources))
	for i := range src.Index.Files {
		p := src.Index.Path(i)
		for si, s := range sources {
			if s.Wants(p) {
				jobs = append(jobs, job{src: s, path: p, file: src.Index.Files[i]})
				matched[si] = append(matched[si], p)
			}
		}
	}

	total := int64(len(jobs))
	step := total / 100
	if step < 1 {
		step = 1
	}
	var done, written, missing int64
	parallel.For(len(jobs), func(k, _ int) {
		j := jobs[k]
		outs, err := j.src.Convert(j.path, j.file)
		if err != nil || len(outs) == 0 {
			atomic.AddInt64(&missing, 1)
		} else {
			dir := filepath.Join(dataDir, j.src.Dir())
			for _, o := range outs {
				dest := filepath.Join(dir, filepath.FromSlash(o.Rel))
				if os.MkdirAll(filepath.Dir(dest), 0o755) != nil {
					continue
				}
				if os.WriteFile(dest, o.Data, 0o644) == nil {
					atomic.AddInt64(&written, 1)
				}
			}
		}
		if n := atomic.AddInt64(&done, 1); n%step == 0 {
			rep.Progress(n, total)
		}
	})
	rep.Progress(total, total)

	// Merge this run's aliases into the shared table rather than overwriting it, so a
	// granular command (e.g. just knowledge icons) doesn't drop another source's
	// entries. Keys are urns, so ownership is identified by the value — each source owns
	// the entries pointing under its Dir/. Clear those and re-add, dropping aliases for
	// items removed since the last run.
	nAlias := 0
	if aliasSources(sources) {
		path := filepath.Join(dataDir, assetRedirectFile)
		redirects := map[string]string{}
		_ = jsonio.ReadFile(path, &redirects) // absent/unreadable -> start empty

		current := make([]map[string]string, len(sources))
		for si, s := range sources {
			current[si] = s.Redirects(matched[si])
		}
		// Prune every contributing source before adding any, since source dirs nest
		// (icons/ contains icons/territories/) — pruning one would otherwise drop
		// another's freshly-added entries depending on iteration order.
		for si, s := range sources {
			if current[si] == nil {
				continue
			}
			prefix := s.Dir() + "/"
			for k, v := range redirects {
				if strings.HasPrefix(v, prefix) {
					delete(redirects, k)
				}
			}
		}
		for _, m := range current {
			for k, v := range m {
				redirects[k] = v
			}
		}
		nAlias = len(redirects)
		if err := jsonio.WriteFile(path, redirects, false); err != nil {
			return err
		}
	}

	names := make([]string, len(sources))
	for i, s := range sources {
		names[i] = s.Name()
	}
	rep.Log(fmt.Sprintf("images %v: wrote %d files, %d aliases (%d entries unconvertible/missing)",
		names, written, nAlias, missing))

	return nil
}

// aliasSources reports whether any source contributes redirect aliases, so runImages
// only reads/rewrites the shared table when there's something to merge.
func aliasSources(sources []imageSource) bool {
	for _, s := range sources {
		if s.Redirects(nil) != nil {
			return true
		}
	}
	return false
}

// decodeTile reads a DDS from the archive and returns its decoded pixels, or nil if
// the file is absent or undecodable. Callers that want pixels (the world-map pyramid
// reuses them for downsampling) use this directly; encodeIcon wraps it for PNG output.
func decodeTile(ar *paz.Archive, f paz.PazFile) *image.NRGBA {
	if f.OrigSize == 0 { // zero-value PazFile = not found
		return nil
	}
	dds, err := ar.Content(f)
	if err != nil {
		return nil
	}
	// Stored entries (CompSize==OrigSize) come back verbatim, and a few DDS textures
	// are stored still-ICE-encrypted. There's no encryption flag in PazFile and Content
	// is format-agnostic, but a plaintext DDS always begins with the "DDS " magic — so
	// branch on it deterministically rather than decoding, failing, then retrying.
	if len(dds) < 4 || string(dds[:4]) != "DDS " {
		if len(dds)%8 == 0 {
			dds = paz.NewICE(paz.BDOICEKey).Decrypt(dds)
		}
	}
	img, err := tex.DecodeDDS(dds)
	if err != nil {
		return nil
	}
	return img
}

// encodeIcon reads a DDS icon from the archive and returns it encoded, or nil if
// the file is absent or undecodable.
func encodeIcon(ar *paz.Archive, f paz.PazFile) []byte {
	img := decodeTile(ar, f)
	if img == nil {
		return nil
	}
	return encodeIconImage(img)
}

// encodeIconImage encodes decoded icon art as WebP, or nil if it won't encode. It
// is the single encoder for everything Icons() writes.
//
// Lossy, deliberately: the icons only ever render at ~20-64px, and lossy is ~40%
// smaller than PNG here. Lossless is not an option — this encoder's VP8L path
// collapses on continuous-tone art (a 128px gradient encodes to ~78x its PNG),
// which would make icons *bigger* than the PNGs they replace.
func encodeIconImage(img *image.NRGBA) []byte {
	var buf bytes.Buffer
	if err := webp.Encode(&buf, img, &webp.EncoderOptions{
		Quality: 50,
		Method:  5,
	}); err != nil {
		return nil
	}
	return buf.Bytes()
}
