package pipeline

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/config"
	"github.com/idevelopthings/bdo-data-extractor/internal/paz"
)

// TextureExtract decodes every .dds whose path contains substr to a PNG under outDir,
// mirroring the archive layout (vs `extract`, which dumps decoded-but-still-DDS bytes).
// With -slice-cell set, each matched atlas is instead cut into a grid of named cells.
func TextureExtract(substr, outDir string) error {
	src, err := paz.OpenSource(*config.GlobalConfig.GameDir)
	if err != nil {
		return err
	}
	defer src.Close()

	if err := src.Archive.AssertSafeOut(outDir); err != nil {
		return err
	}

	spec, err := loadSliceSpec()
	if err != nil {
		return err
	}

	q := strings.ToLower(substr)
	n := 0
	for i, f := range src.Index.Files {
		p := src.Index.Path(i)
		lp := strings.ToLower(p)
		if !strings.HasSuffix(lp, ".dds") || !strings.Contains(lp, q) {
			continue
		}

		img := decodeTile(src.Archive, f)
		if img == nil {
			fmt.Fprintf(os.Stderr, "  skip %s: undecodable\n", p)
			continue
		}

		if spec != nil {
			wrote, err := spec.slice(img, outDir)
			if err != nil {
				return err
			}
			fmt.Printf("sliced %s -> %d cells\n", p, wrote)
			n += wrote
			continue
		}

		dest := filepath.Join(outDir, filepath.FromSlash(strings.TrimSuffix(p, ".dds")+".png"))
		if err := writePNG(dest, img); err != nil {
			return err
		}
		n++
	}

	fmt.Printf("wrote %d PNGs -> %s\n", n, outDir)
	return nil
}

// sliceSpec is a uniform grid cut over an atlas: cols×rows cells of cellW×cellH,
// starting at (originX, originY), each mapped to a row-major name (empty/"-" = skip).
type sliceSpec struct {
	cellW, cellH     int
	originX, originY int
	cols, rows       int
	names            []string
}

// loadSliceSpec builds a sliceSpec from the -slice-* flags, or returns nil when
// slicing wasn't requested (SliceCell empty).
func loadSliceSpec() (*sliceSpec, error) {
	c := config.GlobalConfig
	if c.SliceCell == nil || *c.SliceCell == "" {
		return nil, nil
	}
	s := &sliceSpec{}
	var err error
	if s.cellW, s.cellH, err = parsePair(*c.SliceCell, 'x'); err != nil {
		return nil, fmt.Errorf("-slice-cell: %w", err)
	}
	if s.originX, s.originY, err = parsePair(*c.SliceOrigin, ','); err != nil {
		return nil, fmt.Errorf("-slice-origin: %w", err)
	}
	if s.cols, s.rows, err = parsePair(*c.SliceGrid, 'x'); err != nil {
		return nil, fmt.Errorf("-slice-grid: %w", err)
	}
	if s.names, err = loadNames(*c.SliceNames); err != nil {
		return nil, err
	}
	return s, nil
}

// slice cuts img into the grid and writes one PNG per named cell. Cells whose name
// is empty or "-" are skipped; the grid runs row-major (all of row 0, then row 1…).
func (s *sliceSpec) slice(img image.Image, outDir string) (int, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return 0, err
	}
	wrote := 0
	for r := 0; r < s.rows; r++ {
		for col := 0; col < s.cols; col++ {
			idx := r*s.cols + col
			name := ""
			if idx < len(s.names) {
				name = s.names[idx]
			}
			if name == "" || name == "-" {
				continue
			}
			x0 := s.originX + col*s.cellW
			y0 := s.originY + r*s.cellH
			cell := image.NewNRGBA(image.Rect(0, 0, s.cellW, s.cellH))
			for y := 0; y < s.cellH; y++ {
				for x := 0; x < s.cellW; x++ {
					cell.Set(x, y, img.At(x0+x, y0+y))
				}
			}
			if err := writePNG(filepath.Join(outDir, name+".png"), cell); err != nil {
				return wrote, err
			}
			wrote++
		}
	}
	return wrote, nil
}

// loadNames returns the row-major cell names from either a file (one per line) or a
// comma-separated list. Blank lines and "-" entries are kept as placeholders so the
// grid position stays aligned.
func loadNames(v string) ([]string, error) {
	if v == "" {
		return nil, fmt.Errorf("-slice-names is required when slicing")
	}
	if b, err := os.ReadFile(v); err == nil {
		var out []string
		for _, ln := range strings.Split(string(b), "\n") {
			ln = strings.TrimSpace(strings.TrimPrefix(ln, "\ufeff"))
			if ln == "" || strings.HasPrefix(ln, "#") {
				continue // blank lines/comments are layout only; use "-" to skip a cell
			}
			out = append(out, ln)
		}
		return out, nil
	}
	parts := strings.Split(v, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts, nil
}

func parsePair(s string, sep byte) (int, int, error) {
	i := strings.IndexByte(s, sep)
	if i < 0 {
		return 0, 0, fmt.Errorf("expected A%cB, got %q", sep, s)
	}
	a, err1 := atoiTrim(s[:i])
	b, err2 := atoiTrim(s[i+1:])
	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("invalid number in %q", s)
	}
	return a, b, nil
}

func atoiTrim(s string) (int, error) {
	n := 0
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("not a number: %q", s)
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

func writePNG(dest string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	if err := png.Encode(out, img); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
