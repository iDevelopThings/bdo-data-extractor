package tables

import (
	"image/color"
	"math"
	"sort"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
)

// RegionMap is a decoded BKD region mask (ui_texture/minimap/area/<name>.bmp.bkd):
// a width×height grid of region indices plus per-region metadata resolved from the
// RID sidecar.
type RegionMap struct {
	Name    string       `json:"name"`
	Width   int          `json:"width"`
	Height  int          `json:"height"`
	Regions []RegionCell `json:"regions"`
	Pixels  []uint16     `json:"-"` // row-major region index per pixel (len Width*Height)
}

// RegionCell is one region in a RegionMap: its index, the key/color resolved from
// the RID sidecar, and its pixel-space bounding box + area.
type RegionCell struct {
	Index uint16   `json:"index"`
	Key   uint32   `json:"key,omitempty"`   // region key (RID remap variant)
	Color [3]uint8 `json:"color,omitempty"` // RGB (RID palette variant)
	BBox  [4]int   `json:"bbox"`            // x,y,w,h in pixel space
	Area  int      `json:"area"`
}

// DecodeRegionMap decodes a BKD (+ optional RID sidecar) into a RegionMap.
//
// The BKD is NOT a per-scanline RLE (decoding it as one yields a sheared/diagonal
// image). It is a single FLATTENED row-major run-length stream: bkd-row r owns the
// flat index range [r*65536, (r+1)*65536), within which each span (low16=x0, idx)
// runs to the next span's x0 (or 65536). The concatenated stream is reshaped to the
// true image width, found by autocorrelation.
func DecodeRegionMap(name string, bkd, rid []byte) *RegionMap {
	flat := decodeBKDFlat(bkd)
	if flat == nil {
		return nil
	}
	w := findRegionWidth(flat)
	h := len(flat) / w
	flat = flat[:w*h]

	pal, keys := decodeRID(rid)
	// In a palette map, index 0 is a real region (e.g. the open sea); in a key-remap
	// map, index 0 is nodata (outside the world) and is left out.
	includeZero := len(pal) > 0

	// per-index bounding box + area in one pass (slice keyed by index, no map)
	const maxIdx = 1 << 16
	type acc struct {
		minX, minY, maxX, maxY, area int
		seen                         bool
	}
	accs := make([]acc, maxIdx)
	for i, idx := range flat {
		if idx == 0 && !includeZero {
			continue
		}
		x, y := i%w, i/w
		a := &accs[idx]
		if !a.seen {
			a.seen, a.minX, a.minY, a.maxX, a.maxY = true, x, y, x, y
		} else {
			if x < a.minX {
				a.minX = x
			}
			if x > a.maxX {
				a.maxX = x
			}
			if y < a.minY {
				a.minY = y
			}
			if y > a.maxY {
				a.maxY = y
			}
		}
		a.area++
	}

	rm := &RegionMap{Name: name, Width: w, Height: h, Pixels: flat}
	start := 1
	if includeZero {
		start = 0
	}
	for idx := start; idx < maxIdx; idx++ {
		a := &accs[idx]
		if !a.seen {
			continue
		}
		rc := RegionCell{
			Index: uint16(idx),
			BBox:  [4]int{a.minX, a.minY, a.maxX - a.minX + 1, a.maxY - a.minY + 1},
			Area:  a.area,
		}
		if idx < len(pal) {
			c := pal[idx]
			rc.Color = [3]uint8{c.R, c.G, c.B}
		}
		if idx < len(keys) {
			rc.Key = keys[idx]
		}
		rm.Regions = append(rm.Regions, rc)
	}
	sort.Slice(rm.Regions, func(i, j int) bool { return rm.Regions[i].Index < rm.Regions[j].Index })
	return rm
}

// decodeBKDFlat expands the BKD's run-length spans into one flat row-major index
// array of length rowCount*65536, or nil if the data isn't a valid BKD.
func decodeBKDFlat(bkd []byte) []uint16 {
	n, ok := bss.PABRCount(bkd)
	if !ok {
		return nil
	}
	if n > 1<<20 {
		return nil
	}
	flat := make([]uint16, n*65536)
	p := 8
	for r := 0; r < n; r++ {
		if p+4 > len(bkd) {
			return nil
		}
		sc := int(bss.U32(bkd, p))
		p += 4
		if sc < 0 || p+sc*4 > len(bkd) {
			return nil
		}
		base := r * 65536
		for i := 0; i < sc; i++ {
			v := bss.U32(bkd, p+i*4)
			x0 := int(v & 0xffff)
			idx := uint16(v >> 16)
			x1 := 65536
			if i+1 < sc {
				x1 = int(bss.U32(bkd, p+(i+1)*4) & 0xffff)
			}
			if idx != 0 && x1 > x0 {
				row := flat[base+x0 : base+x1]
				for j := range row {
					row[j] = idx
				}
			}
		}
		p += sc * 4
	}
	return flat
}

// findRegionWidth finds the true image width by autocorrelation: the width W that
// maximizes flat[i]==flat[i+W] (consecutive image rows line up). The image is
// roughly square, so the search starts near sqrt(len).
func findRegionWidth(flat []uint16) int {
	n := len(flat)
	guess := int(math.Sqrt(float64(n)))
	lo, hi := guess*7/10, guess*14/10
	if lo < 2 {
		lo = 2
	}
	L := 2_000_000
	if L > n/4 {
		L = n / 4
	}
	s := n / 3
	for s > 0 && s+L+hi >= n {
		s /= 2
	}

	best, bw := -1, guess
	scan := func(lo, hi, step int) {
		for W := lo; W < hi; W += step {
			if W <= 0 || s+L+W >= n {
				continue
			}
			m := 0
			for i := 0; i < L; i += 16 {
				if flat[s+i] == flat[s+i+W] {
					m++
				}
			}
			if m > best {
				best, bw = m, W
			}
		}
	}
	scan(lo, hi, 8)
	scan(bw-8, bw+9, 1)
	return bw
}

// decodeRID decodes a BKD's .rid sidecar. Two variants, distinguished by size:
// PALETTE = count×4-byte RGBA (region colors); REMAP = count×u16 (region keys).
// Both carry a fixed 51-byte tail. Returns 0-based slices indexed by region index.
func decodeRID(rid []byte) (pal []color.RGBA, keys []uint32) {
	cnt, ok := bss.PABRCount(rid)
	if !ok {
		return nil, nil
	}
	switch {
	case 8+cnt*4+51 == len(rid): // RGBA palette
		pal = make([]color.RGBA, cnt)
		for i := 0; i < cnt; i++ {
			o := 8 + i*4
			pal[i] = color.RGBA{R: rid[o], G: rid[o+1], B: rid[o+2], A: 255}
		}
	case 8+cnt*2+51 == len(rid): // u16 region-key remap
		keys = make([]uint32, cnt)
		for i := 0; i < cnt; i++ {
			keys[i] = uint32(bss.U16(rid, 8+i*2))
		}
	}
	return pal, keys
}
