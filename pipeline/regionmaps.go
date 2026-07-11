package pipeline

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/config"
	"github.com/idevelopthings/bdo-data-extractor/internal/jsonio"
	"github.com/idevelopthings/bdo-data-extractor/internal/paz"
	"github.com/idevelopthings/bdo-data-extractor/internal/progress"
	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
)

// RegionMaps decodes every ui_texture/minimap/area/*.bmp.bkd region mask into a
// downscaled colored PNG + a per-region metadata JSON. Game dir and output dir
// come from the global config.
func RegionMaps() error {
	gameDir := *config.GlobalConfig.GameDir
	outDir := filepath.Join(*config.GlobalConfig.Out, "regionmaps")

	src, err := paz.OpenSource(gameDir)
	if err != nil {
		return err
	}
	defer src.Close()
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	total := int64(0)
	for i := range src.Index.Files {
		p := src.Index.Path(i)
		if strings.Contains(p, "minimap/area") && strings.HasSuffix(p, ".bmp.bkd") {
			total++
		}
	}

	rep := progress.Default()
	n := int64(0)
	for i, f := range src.Index.Files {
		p := src.Index.Path(i)
		if !strings.Contains(p, "minimap/area") || !strings.HasSuffix(p, ".bmp.bkd") {
			continue
		}
		bkd, err := src.Archive.Content(f)
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(p), ".bmp.bkd")
		rid, _ := src.Read(name + ".bmp.rid")

		rm := tables.DecodeRegionMap(name, bkd, rid)
		if rm == nil || len(rm.Regions) == 0 {
			continue
		}

		img := renderRegionMap(rm, 2048)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outDir, name+".png"), buf.Bytes(), 0o644); err != nil {
			return err
		}
		rm.Pixels = nil // don't serialize the raw grid
		if err := jsonio.WriteFile(filepath.Join(outDir, name+".json"), rm, true); err != nil {
			return err
		}
		n++
		rep.Progress(n, total)
	}
	rep.Log(fmt.Sprintf("region maps: %d -> %s", n, outDir))

	return nil
}

// renderRegionMap downscales a RegionMap to at most maxW wide, coloring each region
// by its RID palette color (or a generated hue when the RID is a key remap). Index
// 0 (no region) is left transparent.
func renderRegionMap(rm *tables.RegionMap, maxW int) *image.NRGBA {
	ds := 1
	for rm.Width/ds > maxW {
		ds++
	}
	ow, oh := rm.Width/ds, rm.Height/ds
	img := image.NewNRGBA(image.Rect(0, 0, ow, oh))

	// A region is a palette map if any region carries a non-zero RID color; there,
	// index 0 is a real region and is drawn. In a key-remap map index 0 is nodata
	// and stays transparent.
	palette := false
	for _, rc := range rm.Regions {
		if rc.Color != [3]uint8{} {
			palette = true
			break
		}
	}
	colors := map[uint16]color.NRGBA{}
	for _, rc := range rm.Regions {
		if palette {
			colors[rc.Index] = color.NRGBA{R: rc.Color[0], G: rc.Color[1], B: rc.Color[2], A: 255}
		} else {
			colors[rc.Index] = hueColor(rc.Index)
		}
	}

	for y := 0; y < oh; y++ {
		base := (y * ds) * rm.Width
		for x := 0; x < ow; x++ {
			idx := rm.Pixels[base+x*ds]
			if c, ok := colors[idx]; ok {
				img.SetNRGBA(x, y, c)
			}
		}
	}
	return img
}

// hueColor maps a region index to a distinct color via golden-ratio hue spacing.
func hueColor(i uint16) color.NRGBA {
	h := float64(i) * 0.61803398875
	h -= float64(int(h))
	r, g, b := hsv(h, 0.7, 1.0)
	return color.NRGBA{R: r, G: g, B: b, A: 255}
}

func hsv(h, s, v float64) (uint8, uint8, uint8) {
	i := int(h * 6)
	f := h*6 - float64(i)
	p := v * (1 - s)
	q := v * (1 - f*s)
	t := v * (1 - (1-f)*s)
	var r, g, b float64
	switch i % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	case 5:
		r, g, b = v, p, q
	}
	return uint8(r * 255), uint8(g * 255), uint8(b * 255)
}
