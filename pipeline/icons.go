package pipeline

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/idevelopthings/bdo-data-extractor/internal/config"
	"github.com/idevelopthings/bdo-data-extractor/internal/jsonio"
	"github.com/idevelopthings/bdo-data-extractor/internal/paz"
	"github.com/idevelopthings/bdo-data-extractor/internal/progress"
	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
	"github.com/idevelopthings/bdo-data-extractor/internal/tex"
)

// Icons decodes item icons (DXT-compressed .dds in the PAZ) to PNGs under
// <dataDir>/icons. Items share icons heavily, so each unique icon is decoded and
// written once (named by its icon path); redirects.json then maps every item's
// id-based request path to the shared file. Reads icon paths from items.json (run
// build first); game dir and data dir come from the global config.
func Icons() error {
	gameDir := *config.GlobalConfig.GameDir
	dataDir := *config.GlobalConfig.Out
	outDir := filepath.Join(dataDir, "icons")

	var items []struct {
		ID   uint32 `json:"id"`
		Icon string `json:"icon"`
	}
	if err := jsonio.ReadFile(filepath.Join(dataDir, "items.json"), &items); err != nil {
		return fmt.Errorf("load items.json (run build first): %w", err)
	}

	src, err := paz.OpenSource(gameDir)
	if err != nil {
		return err
	}
	defer src.Close()
	if err := src.Archive.AssertSafeOut(outDir); err != nil {
		return err
	}

	// Rewrite the icons dir from scratch: the shared-file naming means old per-id
	// files (or an icon that changed between patches) would otherwise linger.
	if err := os.RemoveAll(outDir); err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	// index icon files by lowercased archive path
	files := make(map[string]paz.PazFile)
	for i, f := range src.Index.Files {
		p := strings.ToLower(src.Index.Path(i))
		if strings.HasSuffix(p, ".dds") && strings.Contains(p, "icon/new_icon/") {
			files[p] = f
		}
	}

	// Map each unique icon (by its normalized path slug) to the archive entry to
	// decode, and record a redirect from every item's id-based request path to the
	// shared file. Keys/targets are relative to the served data dir.
	archiveOf := map[string]string{}
	redirects := map[string]string{}
	for _, it := range items {
		if it.Icon == "" {
			continue
		}
		low := strings.ReplaceAll(strings.ToLower(it.Icon), "\\", "/")
		slug := strings.TrimSuffix(low, ".dds") + ".png"
		archiveOf[slug] = "ui_texture/icon/" + low
		redirects[fmt.Sprintf("icons/%d.png", it.ID)] = "icons/" + slug
	}

	total := int64(len(archiveOf))
	step := total / 50
	if step < 1 {
		step = 1
	}

	// decode + write in parallel: DXT decoding is CPU-bound, writes are IO-bound,
	// and the archive reader is concurrency-safe.
	tasks := make(chan string, len(archiveOf))
	for slug := range archiveOf {
		tasks <- slug
	}
	close(tasks)

	rep := progress.Default()
	var written, missing int64
	var wg sync.WaitGroup
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for slug := range tasks {
				data := encodeIcon(src.Archive, files[archiveOf[slug]])
				if data == nil {
					atomic.AddInt64(&missing, 1)
					continue
				}
				dest := filepath.Join(outDir, filepath.FromSlash(slug))
				if os.MkdirAll(filepath.Dir(dest), 0o755) != nil {
					continue
				}
				if os.WriteFile(dest, data, 0o644) == nil {
					if n := atomic.AddInt64(&written, 1); n%step == 0 {
						rep.Progress(n, total)
					}
				}
			}
		}()
	}
	wg.Wait()
	rep.Progress(atomic.LoadInt64(&written), total)

	if err := jsonio.WriteFile(filepath.Join(outDir, "redirects.json"), redirects, false); err != nil {
		return err
	}
	rep.Log(fmt.Sprintf("wrote %d unique icons, %d item redirects -> %s (%d unconvertible/missing)", written, len(redirects), outDir, missing))

	if n, err := extractZoneCategoryIcons(src, dataDir, outDir); err != nil {
		rep.Log(fmt.Sprintf("zone-category icons: %v", err))
	} else if n > 0 {
		rep.Log(fmt.Sprintf("wrote %d zone-category icons -> %s", n, filepath.Join(outDir, "zonecategories")))
	}
	if n, err := extractTerritoryIcons(src, dataDir, outDir); err != nil {
		rep.Log(fmt.Sprintf("territory icons: %v", err))
	} else if n > 0 {
		rep.Log(fmt.Sprintf("wrote %d territory icons -> %s", n, filepath.Join(outDir, "territories")))
	}
	return nil
}

// extractTerritoryIcons decodes each territory's worldmap-mark textures to PNGs
// under outDir/territories, named as world.json's iconLarge/iconSmall paths
// expect. The .dds paths come straight from territoryinfo.bss (world.json holds
// the final PNG paths, not the texture paths).
func extractTerritoryIcons(src *paz.Source, dataDir, outDir string) (int, error) {
	raw, err := src.Read("territoryinfo.bss")
	if err != nil {
		return 0, err
	}
	terrs, err := tables.DecodeTerritories(raw)
	if err != nil {
		return 0, err
	}
	want := map[string]string{} // lowercased archive path -> output file name
	for _, t := range terrs {
		for _, icon := range []string{t.IconLarge, t.IconSmall} {
			if icon != "" {
				want[tables.TerritoryIconArchivePath(icon)] = tables.TerritoryIconFile(icon)
			}
		}
	}
	files := map[string]paz.PazFile{}
	for i, f := range src.Index.Files {
		p := strings.ToLower(src.Index.Path(i))
		if _, ok := want[p]; ok {
			files[p] = f
		}
	}
	terrDir := filepath.Join(outDir, "territories")
	if err := os.MkdirAll(terrDir, 0o755); err != nil {
		return 0, err
	}
	written := 0
	for p, name := range want {
		data := encodeIcon(src.Archive, files[p])
		if data == nil {
			progress.Default().Log(fmt.Sprintf("  territory icon missing/undecodable: %s", p))
			continue
		}
		if err := os.WriteFile(filepath.Join(terrDir, name), data, 0o644); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

// zoneCategoryAtlas is the UI sprite sheet holding the Monster Zone Info tab and
// filter-button icons (the icon ids stored in zones.json).
const zoneCategoryAtlas = "ui_texture/combine/etc/combine_etc_dropitem.dds"

// atlasGrid describes a family of uniformly-packed sprites in the atlas: slot 1's
// top-left, the cell size, the horizontal stride between slots, and the vertical
// stride between states (Normal/Over/Click). The per-slot UVs are computed by the
// UI script at runtime (only one template slot is in the XML), so the grid is
// measured from the atlas and verified to reproduce the template UVs exactly.
type atlasGrid struct {
	x0, y0, w, h, hStride, vStride int
}

var zoneIconGrids = map[string]atlasGrid{
	"tab":        {x0: 1, y0: 1, w: 45, h: 45, hStride: 46, vStride: 46},     // region tabs
	"filter_btn": {x0: 240, y0: 314, w: 42, h: 42, hStride: 43, vStride: 43}, // content filters
}

var iconStates = []struct {
	suffix string
	idx    int
}{{"_normal", 0}, {"_over", 1}, {"_click", 2}}

// zoneIconRect maps an icon id like "Combine_Etc_DropItem_Icon_Tab_05" or
// "..._Filter_Btn_05_Over" to its rectangle in the atlas. Main-category ids carry
// no state suffix (normal); sub-category ids end in _Over.
func zoneIconRect(name string) (image.Rectangle, bool) {
	const prefix = "combine_etc_dropitem_icon_"
	low := strings.ToLower(name)
	i := strings.Index(low, prefix)
	if i < 0 {
		return image.Rectangle{}, false
	}
	rest := low[i+len(prefix):] // e.g. "tab_05" or "filter_btn_05_over"
	state := 0
	for _, s := range iconStates {
		if strings.HasSuffix(rest, s.suffix) {
			state, rest = s.idx, strings.TrimSuffix(rest, s.suffix)
			break
		}
	}
	j := strings.LastIndex(rest, "_")
	if j < 0 {
		return image.Rectangle{}, false
	}
	g, ok := zoneIconGrids[rest[:j]]
	num, err := strconv.Atoi(rest[j+1:])
	if !ok || err != nil || num < 1 {
		return image.Rectangle{}, false
	}
	x := g.x0 + (num-1)*g.hStride
	y := g.y0 + state*g.vStride
	return image.Rect(x, y, x+g.w, y+g.h), true
}

// extractZoneCategoryIcons crops each Monster Zone Info main/sub-category icon
// out of its UI atlas and writes it as <iconId>.png under outDir/zonecategories,
// where <iconId> is the icon field from zones.json. Mirrors the item-icon path:
// one DDS decode, then per-sprite PNGs.
func extractZoneCategoryIcons(src *paz.Source, dataDir, outDir string) (int, error) {
	var zones []struct {
		MainCategory *struct {
			Icon string `json:"icon"`
		} `json:"mainCategory"`
		SubCategories []struct {
			Icon string `json:"icon"`
		} `json:"subCategories"`
	}
	if err := jsonio.ReadFile(filepath.Join(dataDir, "zones.json"), &zones); err != nil {
		return 0, nil // zones.json not built yet — nothing to do
	}
	names := map[string]bool{}
	for _, z := range zones {
		if z.MainCategory != nil && z.MainCategory.Icon != "" {
			names[z.MainCategory.Icon] = true
		}
		for _, s := range z.SubCategories {
			if s.Icon != "" {
				names[s.Icon] = true
			}
		}
	}
	if len(names) == 0 {
		return 0, nil
	}

	var atlasFile paz.PazFile
	for i := range src.Index.Files {
		if strings.ToLower(src.Index.Path(i)) == zoneCategoryAtlas {
			atlasFile = src.Index.Files[i]
			break
		}
	}
	if atlasFile.OrigSize == 0 {
		return 0, fmt.Errorf("atlas %s not found", zoneCategoryAtlas)
	}
	dds, err := src.Archive.Content(atlasFile)
	if err != nil {
		return 0, err
	}
	atlas, err := tex.DecodeDDS(dds)
	if err != nil {
		return 0, err
	}

	catDir := filepath.Join(outDir, "zonecategories")
	if err := os.MkdirAll(catDir, 0o755); err != nil {
		return 0, err
	}
	// each icon has 3 state rows in the atlas: Normal (<id>.png, the resting bar
	// look), Over (<id>_Over.png) and Click (<id>_Click.png).
	written := 0
	for name := range names {
		for _, suffix := range []string{"", "_Over", "_Click"} {
			rect, ok := zoneIconRect(name + suffix)
			if !ok {
				continue
			}
			sub, ok := atlas.SubImage(rect).(*image.NRGBA)
			if !ok || sub.Bounds().Empty() {
				continue
			}
			var buf bytes.Buffer
			if png.Encode(&buf, sub) != nil {
				continue
			}
			if err := os.WriteFile(filepath.Join(catDir, name+suffix+".png"), buf.Bytes(), 0o644); err != nil {
				return written, err
			}
			written++
		}
	}
	return written, nil
}

// encodeIcon reads a DDS icon from the archive and returns it as PNG bytes,
// or nil if the file is absent or undecodable.
func encodeIcon(ar *paz.Archive, f paz.PazFile) []byte {
	if f.OrigSize == 0 { // zero-value PazFile = not found
		return nil
	}
	dds, err := ar.Content(f)
	if err != nil {
		return nil
	}
	img, err := tex.DecodeDDS(dds)
	if err != nil && len(dds)%8 == 0 {
		// a few textures are stored uncompressed but still ICE-encrypted
		// (Content only decrypts compressed entries) — retry decrypted
		img, err = tex.DecodeDDS(paz.NewICE(paz.BDOICEKey).Decrypt(dds))
	}
	if err != nil {
		return nil
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return buf.Bytes()
}
