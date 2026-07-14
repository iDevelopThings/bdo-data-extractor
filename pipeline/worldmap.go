package pipeline

import (
	"bytes"
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/deepteams/webp"
	"github.com/dgravesa/go-parallel/parallel"

	"github.com/idevelopthings/bdo-data-extractor/internal/config"
	"github.com/idevelopthings/bdo-data-extractor/internal/jsonio"
	"github.com/idevelopthings/bdo-data-extractor/internal/paz"
	"github.com/idevelopthings/bdo-data-extractor/internal/progress"
	"github.com/idevelopthings/bdo-data-extractor/src/utils"
)

// The radar tiles cover the world at 100 game-units per pixel; a 128px tile is one
// 12,800-unit chunk. The viewer reads these constants back from meta.json to place
// tiles in world space, so they live with the pyramid, not hard-coded downstream.
const worldUnitsPerPixel = 100

// tileCoord is a tile's (x, y) grid index. Indices are absolute (origin at world 0,0)
// and can be negative; because levels only halve, floorDiv2 walks child->parent.
type tileCoord = [2]int

// native is one source radar tile: its archive record and parsed grid coordinate.
type native struct {
	file paz.PazFile
	x, y int
}

// WorldMap decodes the minimap_data radar tiles into a slippy-style zoom pyramid at
// worldmap/<layer>/<z>/<x>/<y>.webp with a per-layer meta.json. It builds the world and
// Land of the Morning Light layers (the pack layer duplicates world and is skipped).
// Each native DDS is decoded once: its pixels are WebP-encoded for the finest level and
// box-downsampled in memory to compose every coarser level, so no tile is written then
// read back. It replaces the old flat single-resolution grid and runs standalone rather
// than through runImages — the mip cascade doesn't fit the one-file-per-job model.
func WorldMap() error {
	gameDir := *config.GlobalConfig.GameDir
	dataDir := *config.GlobalConfig.Out

	timed := utils.Timed("World Map")
	defer timed()

	src, err := paz.OpenSource(gameDir)
	if err != nil {
		return err
	}
	defer src.Close()

	outRoot := filepath.Join(dataDir, "worldmap")
	if err := src.Archive.AssertSafeOut(outRoot); err != nil {
		return err
	}

	// One index pass buckets the native tiles by layer.
	natives := map[string][]native{}
	for i := range src.Index.Files {
		layer, x, y, ok := worldNativeAt(src.Index.Path(i))
		if !ok {
			continue
		}
		natives[layer] = append(natives[layer], native{file: src.Index.Files[i], x: x, y: y})
	}

	// Rebuild from scratch — coarse levels are shared-named and would otherwise leave
	// stale tiles behind across a patch.
	if err := os.RemoveAll(outRoot); err != nil {
		return err
	}
	if err := os.MkdirAll(outRoot, 0o755); err != nil {
		return err
	}

	rep := progress.Default()
	for _, layer := range []string{"world", "morningland"} {
		tiles := natives[layer]
		if layer == "world" {
			tiles = cropTiles(tiles, worldCrop.xmin, worldCrop.xmax, worldCrop.ymin, worldCrop.ymax)
		}
		if len(tiles) == 0 {
			continue
		}
		if err := buildLayer(src, filepath.Join(outRoot, layer), layer, tiles, rep); err != nil {
			return err
		}
	}
	return nil
}

// worldCrop bounds the world layer to the live playable region, dropping the far
// WIP/dev frontier (the untextured snow expanse in the far NW) and stray edge tiles that
// balloon the map far past what players see. Native tile coords; morningland is already tight.
var worldCrop = struct{ xmin, xmax, ymin, ymax int }{xmin: -128, xmax: 111, ymin: -64, ymax: 126}

// cropTiles keeps only the tiles inside [xmin,xmax]×[ymin,ymax].
func cropTiles(tiles []native, xmin, xmax, ymin, ymax int) []native {
	out := tiles[:0]
	for _, t := range tiles {
		if t.x >= xmin && t.x <= xmax && t.y >= ymin && t.y <= ymax {
			out = append(out, t)
		}
	}
	return out
}

// worldNativeAt classifies a minimap_data archive path, returning the layer and the
// tile's grid coordinate. ok is false for anything that isn't a world/morningland radar
// tile — including the pack duplicate, which we deliberately skip. The leaf is always
// Rader_<x>_<y>.dds; the layer comes from the parent folder (the base grid in
// minimap_data/, morningland under minimap_data/_morningland/, pack in the sibling
// minimap_data_pack/).
func worldNativeAt(path string) (layer string, x, y int, ok bool) {
	p := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	if !strings.Contains(p, "minimap_data") || !strings.HasSuffix(p, ".dds") {
		return "", 0, 0, false
	}
	i := strings.LastIndexByte(p, '/')
	if i < 0 {
		return "", 0, 0, false
	}
	dir, leaf := p[:i], p[i+1:]
	switch {
	case strings.HasSuffix(dir, "_morningland"):
		layer = "morningland"
	case strings.HasSuffix(dir, "minimap_data_pack"):
		return "", 0, 0, false // pack duplicates world
	default:
		layer = "world"
	}
	coord := strings.TrimSuffix(strings.TrimPrefix(leaf, "rader_"), ".dds")
	xs, ys, found := strings.Cut(coord, "_")
	if !found {
		return "", 0, 0, false
	}
	x, err1 := strconv.Atoi(xs)
	y, err2 := strconv.Atoi(ys)
	if err1 != nil || err2 != nil {
		return "", 0, 0, false
	}
	return layer, x, y, true
}

// buildLayer decodes one layer's natives into its finest level and composes the coarser
// levels in memory down to z=0, then writes the layer's meta.json.
func buildLayer(src *paz.Source, dir, layer string, tiles []native, rep progress.Reporter) error {
	xmin, xmax, ymin, ymax := tiles[0].x, tiles[0].x, tiles[0].y, tiles[0].y
	for _, t := range tiles {
		xmin, xmax = min(xmin, t.x), max(xmax, t.x)
		ymin, ymax = min(ymin, t.y), max(ymax, t.y)
	}
	span := max(xmax-xmin+1, ymax-ymin+1)
	zmax := max(int(math.Ceil(math.Log2(float64(span)))), 0)

	// Tile size is derived from the first decodable native rather than assumed, then
	// every other tile is checked against it (any mismatch is dropped, not composited).
	tilePx := 0
	for _, t := range tiles {
		if img := decodeTile(src.Archive, t.file); img != nil {
			tilePx = img.Bounds().Dx()
			break
		}
	}
	if tilePx == 0 {
		return fmt.Errorf("world map %s: no decodable tiles", layer)
	}

	rep.Phase(fmt.Sprintf("world map %s: %d tiles, z0..%d", layer, len(tiles), zmax))

	// Fill every empty cell in the (cropped) bounding box with one representative open-water
	// tile so the sea is continuous and the pyramid dense (no holes at any zoom). It's a real
	// textured radar tile, encoded once and shared across every fill cell — not a flat colour.
	oceanImg := detectOceanTile(src.Archive, tiles, tilePx)
	var oceanBytes []byte
	var oceanCoords [][2]int
	var oceanColor [3]int
	if oceanImg != nil {
		oceanBytes, _ = encodeTile(oceanImg)
		mr, mg, mb, _ := tileStats(oceanImg)
		oceanColor = [3]int{int(mr), int(mg), int(mb)}
		realSet := make(map[tileCoord]bool, len(tiles))
		for _, t := range tiles {
			realSet[tileCoord{t.x, t.y}] = true
		}
		for y := ymin; y <= ymax; y++ {
			for x := xmin; x <= xmax; x++ {
				if !realSet[tileCoord{x, y}] {
					oceanCoords = append(oceanCoords, [2]int{x, y})
				}
			}
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	pack, err := newTilePack(filepath.Join(dir, tilePackName))
	if err != nil {
		return err
	}

	// Phase 1: decode every native, WebP the finest tile, and box-downsample into its
	// z=zmax-1 parent. Each worker owns its own parent map keyed by parallel.For's
	// goroutine id, so composition needs no locking; the disjoint maps merge afterwards.
	nWorkers := parallel.DefaultNumGoroutines()
	perWorker := make([]map[tileCoord]*image.NRGBA, nWorkers)
	for i := range perWorker {
		perWorker[i] = map[tileCoord]*image.NRGBA{}
	}

	total := int64(len(tiles))
	step := max(total/500, 1)
	var written, missing, done int64

	parallel.For(len(tiles), func(k, grID int) {
		t := tiles[k]
		img := decodeTile(src.Archive, t.file)
		if img == nil || img.Bounds().Dx() != tilePx || img.Bounds().Dy() != tilePx {
			atomic.AddInt64(&missing, 1)
		} else {
			if data, err := encodeTile(img); err == nil && pack.add(zmax, t.x, t.y, data) == nil {
				atomic.AddInt64(&written, 1)
			}
			if zmax > 0 {
				accumulate(perWorker[grID], img, t.x, t.y, tilePx)
			}
		}
		if n := atomic.AddInt64(&done, 1); n%step == 0 {
			rep.Progress(n, total)
		}
	})
	rep.Progress(total, total)

	// Merge the per-worker z=zmax-1 tiles. Two workers can hold different quadrants of
	// the same parent, so compose them opaquely rather than overwrite.
	level := map[tileCoord]*image.NRGBA{}
	for _, m := range perWorker {
		for p, t := range m {
			if dst := level[p]; dst != nil {
				overlay(dst, t)
			} else {
				level[p] = t
			}
		}
	}
	perWorker = nil

	// Fill the sea: one shared finest blob for every empty cell (deduped in the pack), and
	// downsample it into z=zmax-1 so coarse zooms show continuous water too.
	if oceanImg != nil {
		pack.addMany(zmax, oceanCoords, oceanBytes)
		if zmax > 0 {
			for _, c := range oceanCoords {
				accumulate(level, oceanImg, c[0], c[1], tilePx)
			}
		}
	}

	// Phase 2: write each level, then compose the next-coarser one from it in memory,
	// down to z=0.
	var coarse int64
	for z := zmax - 1; z >= 0; z-- {
		coarse += writeLevel(pack, z, level)
		if z == 0 {
			break
		}
		next := map[tileCoord]*image.NRGBA{}
		for c, t := range level {
			p := tileCoord{floorDiv2(c[0]), floorDiv2(c[1])}
			parent := next[p]
			if parent == nil {
				parent = image.NewNRGBA(image.Rect(0, 0, tilePx, tilePx))
				next[p] = parent
			}
			place(parent, t, c[0]-2*p[0], c[1]-2*p[1])
		}
		level = next
	}

	if err := pack.close(); err != nil {
		return err
	}
	if err := writeLayerMeta(dir, tilePx, zmax, xmin, xmax, ymin, ymax, oceanColor); err != nil {
		return err
	}
	rep.Log(fmt.Sprintf("world map %s: %d natives -> z0..%d, %d finest + %d ocean-fill + %d coarse (%d unusable)",
		layer, len(tiles), zmax, written, len(oceanCoords), coarse, missing))
	return nil
}

// writeLevel WebP-encodes a whole level's tiles in parallel and appends them to the
// pack, returning the count written. Encode failures are dropped (reflected in the
// count) rather than aborting the run, matching the icon pipeline.
func writeLevel(pack *tilePack, z int, tiles map[tileCoord]*image.NRGBA) int64 {
	coords := make([]tileCoord, 0, len(tiles))
	for c := range tiles {
		coords = append(coords, c)
	}
	var written int64
	parallel.For(len(coords), func(k, _ int) {
		c := coords[k]
		if data, err := encodeTile(tiles[c]); err == nil && pack.add(z, c[0], c[1], data) == nil {
			atomic.AddInt64(&written, 1)
		}
	})
	return written
}

// encodeTile encodes a tile as lossy WebP. Map imagery compresses far smaller than PNG
// this way, the alpha channel (transparent quadrants over ocean gaps) is preserved, and
// browsers decode WebP natively so the viewer needs no extra decoder.
func encodeTile(img *image.NRGBA) ([]byte, error) {
	var buf bytes.Buffer
	if err := webp.Encode(&buf, img, &webp.EncoderOptions{Quality: 80, Method: 2}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// floorDiv2 halves a coordinate toward negative infinity; Go's / truncates toward zero,
// which would break the child->parent mapping on the negative half of the map.
func floorDiv2(a int) int {
	if a >= 0 {
		return a / 2
	}
	return (a - 1) / 2
}

// place box-averages child (2×2 → 1) straight into its quadrant of parent, writing to
// the parent's pixel bytes directly — no intermediate image and no draw.Draw, both of
// which dominated the profile. qx selects west(0)/east(1); qy selects south(0)/north(1),
// so the north child lands in the top half (native north-up, image row 0 = north edge).
func place(parent, child *image.NRGBA, qx, qy int) {
	hw, hh := child.Rect.Dx()/2, child.Rect.Dy()/2 // half size = destination quadrant size
	ox, oy := qx*hw, (1-qy)*hh                     // top-left of the quadrant in parent
	cp, pp := child.Pix, parent.Pix
	for y := 0; y < hh; y++ {
		s0 := child.PixOffset(0, y*2)
		s1 := s0 + child.Stride
		d := parent.PixOffset(ox, oy+y)
		for x := 0; x < hw; x++ {
			pp[d+0] = byte((int(cp[s0+0]) + int(cp[s0+4]) + int(cp[s1+0]) + int(cp[s1+4])) / 4)
			pp[d+1] = byte((int(cp[s0+1]) + int(cp[s0+5]) + int(cp[s1+1]) + int(cp[s1+5])) / 4)
			pp[d+2] = byte((int(cp[s0+2]) + int(cp[s0+6]) + int(cp[s1+2]) + int(cp[s1+6])) / 4)
			pp[d+3] = byte((int(cp[s0+3]) + int(cp[s0+7]) + int(cp[s1+3]) + int(cp[s1+7])) / 4)
			s0 += 8
			s1 += 8
			d += 4
		}
	}
}

// overlay copies src's opaque pixels onto dst (same dimensions). Tiles are binary-alpha
// — a quadrant is either fully opaque (a child filled it) or fully transparent — so a
// copy-where-opaque is exactly draw.Over here, without its per-pixel blend machinery.
func overlay(dst, src *image.NRGBA) {
	dp, sp := dst.Pix, src.Pix
	for i := 0; i+3 < len(sp); i += 4 {
		if sp[i+3] != 0 {
			dp[i+0] = sp[i+0]
			dp[i+1] = sp[i+1]
			dp[i+2] = sp[i+2]
			dp[i+3] = sp[i+3]
		}
	}
}

// accumulate box-downsamples child into its z-1 parent within m, creating the parent on
// first touch. The child at (x,y) lands in parent floorDiv2(x,y)'s quadrant.
func accumulate(m map[tileCoord]*image.NRGBA, child *image.NRGBA, x, y, tilePx int) {
	p := tileCoord{floorDiv2(x), floorDiv2(y)}
	parent := m[p]
	if parent == nil {
		parent = image.NewNRGBA(image.Rect(0, 0, tilePx, tilePx))
		m[p] = parent
	}
	place(parent, child, x-2*p[0], y-2*p[1])
}

// detectOceanTile samples the natives and returns a representative open-water tile — the
// lowest-variance dark, bluish one — to fill cells the radar has no tile for. Returns nil
// if nothing looks like water (then the sea is just left sparse).
func detectOceanTile(ar *paz.Archive, tiles []native, tilePx int) *image.NRGBA {
	const samples = 600
	stride := max(len(tiles)/samples, 1)
	var best *image.NRGBA
	bestVar := math.MaxFloat64
	for i := 0; i < len(tiles); i += stride {
		img := decodeTile(ar, tiles[i].file)
		if img == nil || img.Bounds().Dx() != tilePx || img.Bounds().Dy() != tilePx {
			continue
		}
		mr, mg, mb, v := tileStats(img)
		// Water is dark and bluish/teal (blue ≥ red, green ≥ red); this rejects the other
		// low-variance tiles — bright snow and tan desert.
		if (mr+mg+mb)/3 < 120 && mb >= mr && mg >= mr && v < bestVar {
			bestVar, best = v, img
		}
	}
	return best
}

// tileStats returns the per-channel means and a cheap total variance over a subsample.
func tileStats(img *image.NRGBA) (mr, mg, mb, variance float64) {
	p := img.Pix
	var sr, sg, sb, sr2, sg2, sb2 float64
	n := 0
	for i := 0; i+3 < len(p); i += 16 { // every 4th pixel
		r, g, b := float64(p[i]), float64(p[i+1]), float64(p[i+2])
		sr, sg, sb = sr+r, sg+g, sb+b
		sr2, sg2, sb2 = sr2+r*r, sg2+g*g, sb2+b*b
		n++
	}
	if n == 0 {
		return 0, 0, 0, math.MaxFloat64
	}
	fn := float64(n)
	mr, mg, mb = sr/fn, sg/fn, sb/fn
	variance = (sr2/fn - mr*mr) + (sg2/fn - mg*mg) + (sb2/fn - mb*mb)
	return
}

// worldMeta is the per-layer descriptor the viewer reads to place tiles in world space.
type worldMeta struct {
	TilePx          int `json:"tilePx"`
	UnitsPerPixel   int `json:"unitsPerPixel"`
	UnitsPerTile    int `json:"unitsPerTile"`
	MinZoom         int `json:"minZoom"`
	MaxZoom         int `json:"maxZoom"`
	TileWorldSizeZ0 int `json:"tileWorldSizeZ0"`
	Grid            struct {
		XMin int `json:"xmin"`
		XMax int `json:"xmax"`
		YMin int `json:"ymin"`
		YMax int `json:"ymax"`
	} `json:"grid"`
	// OceanColor is the fill tile's mean RGB — the viewer paints its backdrop this color
	// so the sea reads as continuous beyond the tiled extent.
	OceanColor [3]int `json:"oceanColor"`
}

func writeLayerMeta(dir string, tilePx, zmax, xmin, xmax, ymin, ymax int, oceanColor [3]int) error {
	unitsPerTile := worldUnitsPerPixel * tilePx
	m := worldMeta{
		TilePx:          tilePx,
		UnitsPerPixel:   worldUnitsPerPixel,
		UnitsPerTile:    unitsPerTile,
		MinZoom:         0,
		MaxZoom:         zmax,
		TileWorldSizeZ0: unitsPerTile << zmax,
		OceanColor:      oceanColor,
	}
	m.Grid.XMin, m.Grid.XMax = xmin, xmax
	m.Grid.YMin, m.Grid.YMax = ymin, ymax
	return jsonio.WriteFile(filepath.Join(dir, "meta.json"), m, true)
}
