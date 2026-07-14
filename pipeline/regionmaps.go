package pipeline

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/jsonio"
	"github.com/idevelopthings/bdo-data-extractor/internal/paz"
	"github.com/idevelopthings/bdo-data-extractor/internal/tables"
)

// --- region maps ------------------------------------------------------------

// regionMap decodes every ui_texture/minimap/area/*.bmp.bkd region mask into a
// downscaled colored PNG plus a per-region metadata JSON under regionmaps/.
type regionMap struct {
	base
}

func regionMaps() imageSource {
	return &regionMap{}
}

func (s *regionMap) Name() string { return "region maps" }
func (s *regionMap) Dir() string  { return "regionmaps" }

func (s *regionMap) Prepare(src *paz.Source, dataDir string) error {
	s.src = src
	return nil
}

func (s *regionMap) Wants(path string) bool {
	return strings.Contains(path, "minimap/area") && strings.HasSuffix(path, ".bmp.bkd")
}

func (s *regionMap) Convert(path string, f paz.PazFile) ([]output, error) {
	bkd, err := s.src.Archive.Content(f)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSuffix(filepath.Base(path), ".bmp.bkd")
	rid, _ := s.src.Read(name + ".bmp.rid")

	rm := tables.DecodeRegionMap(name, bkd, rid)
	if rm == nil || len(rm.Regions) == 0 {
		return nil, nil
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, renderRegionMap(rm, 2048)); err != nil {
		return nil, err
	}
	rm.Pixels = nil // don't serialize the raw grid
	meta, err := jsonio.Marshal(rm, true)
	if err != nil {
		return nil, err
	}
	return []output{
		{Rel: name + ".png", Data: buf.Bytes()},
		{Rel: name + ".json", Data: meta},
	}, nil
}

// Should be kept for now, useful for debugging/pulling files to see whats available
type worldMiniMap struct {
	base
}

func worldMiniMaps() imageSource {
	return &worldMiniMap{}
}

func (s *worldMiniMap) Name() string  { return "world mini map" }
func (s *worldMiniMap) Dir() string   { return "worldminimap" }
func (s *worldMiniMap) Rebuild() bool { return true }

func (s *worldMiniMap) Prepare(src *paz.Source, dataDir string) error {
	s.src = src
	return nil
}

func (s *worldMiniMap) Wants(path string) bool {
	return strings.Contains(path, "new_ui_common_forlua") &&
		!strings.Contains(path, "rader") &&
		strings.HasSuffix(path, ".dds")

	// return strings.Contains(path, "minimap") && strings.HasSuffix(path, ".dds")
}

func (s *worldMiniMap) Convert(path string, f paz.PazFile) ([]output, error) {
	data := encodeIcon(s.src.Archive, f)
	if data == nil {
		return nil, nil
	}
	fileName := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	fileName = strings.ReplaceAll(fileName, "/", "_")
	fileName = strings.TrimSuffix(fileName, ".dds") + ".png"
	fileName = "minimap/" + fileName
	return []output{{Rel: fileName, Data: data}}, nil
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
