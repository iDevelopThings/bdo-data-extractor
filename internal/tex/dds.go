// Package tex decodes the DDS textures used by BDO icons into Go images.
package tex

import (
	"fmt"
	"image"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
)

// DecodeDDS decodes a DDS texture (mip 0) into a non-premultiplied NRGBA image.
// DDS stores straight (non-premultiplied) alpha, so NRGBA is the correct model —
// using image.RGBA (premultiplied) would corrupt semi-transparent edge pixels on
// re-encode. Supports the DXT1 (BC1) and DXT5 (BC3) compressed formats used by
// BDO's New_Icon set, plus uncompressed 32-bit BGRA. Errors for anything else.
func DecodeDDS(b []byte) (*image.NRGBA, error) {
	if len(b) < 128 || string(b[:4]) != "DDS " {
		return nil, fmt.Errorf("not a DDS file")
	}
	h := int(bss.U32(b, 12))
	w := int(bss.U32(b, 16))
	if w <= 0 || h <= 0 || w > 8192 || h > 8192 {
		return nil, fmt.Errorf("bad DDS dimensions %dx%d", w, h)
	}
	fourCC := string(b[84:88])
	data := b[128:]
	img := image.NewNRGBA(image.Rect(0, 0, w, h))

	switch fourCC {
	case "DXT1", "DXT5":
		blockBytes := 16
		if fourCC == "DXT1" {
			blockBytes = 8
		}
		bx, by := (w+3)/4, (h+3)/4
		if len(data) < bx*by*blockBytes {
			return nil, fmt.Errorf("DDS %s data truncated", fourCC)
		}
		pos := 0
		for y := 0; y < by; y++ {
			for x := 0; x < bx; x++ {
				if fourCC == "DXT5" {
					decodeBC3Block(data[pos:pos+16], img, x*4, y*4)
				} else {
					decodeBC1Block(data[pos:pos+8], img, x*4, y*4)
				}
				pos += blockBytes
			}
		}
		return img, nil
	default:
		// uncompressed 32-bit (flags has RGB bit, 32 bpp) — assume BGRA
		bpp := int(bss.U32(b, 88))
		if bpp == 32 && len(data) >= w*h*4 {
			for i := 0; i < w*h; i++ {
				img.Pix[i*4+0] = data[i*4+2] // R <- B
				img.Pix[i*4+1] = data[i*4+1]
				img.Pix[i*4+2] = data[i*4+0] // B <- R
				img.Pix[i*4+3] = data[i*4+3]
			}
			return img, nil
		}
		return nil, fmt.Errorf("unsupported DDS format %q", fourCC)
	}
}

func rgb565(c uint16) (r, g, b uint8) {
	r = uint8((c>>11)&0x1f) << 3
	g = uint8((c>>5)&0x3f) << 2
	b = uint8(c&0x1f) << 3
	r |= r >> 5
	g |= g >> 6
	b |= b >> 5
	return
}

// colorPalette builds the 4 interpolated colors of a BC1/BC3 color block.
// opaque4 forces the 4-color mode (always true for BC3).
func colorPalette(block []byte, opaque4 bool) [4][3]uint8 {
	c0 := bss.U16(block, 0)
	c1 := bss.U16(block, 2)
	var p [4][3]uint8
	p[0][0], p[0][1], p[0][2] = rgb565(c0)
	p[1][0], p[1][1], p[1][2] = rgb565(c1)
	if opaque4 || c0 > c1 {
		for i := 0; i < 3; i++ {
			p[2][i] = uint8((2*int(p[0][i]) + int(p[1][i])) / 3)
			p[3][i] = uint8((int(p[0][i]) + 2*int(p[1][i])) / 3)
		}
	} else {
		for i := 0; i < 3; i++ {
			p[2][i] = uint8((int(p[0][i]) + int(p[1][i])) / 2)
		}
		// p[3] stays {0,0,0}; index 3 is transparent in DXT1's 3-color mode
	}
	return p
}

func decodeBC1Block(block []byte, img *image.NRGBA, ox, oy int) {
	transparent := bss.U16(block, 0) <= bss.U16(block, 2)
	pal := colorPalette(block, false)
	bits := bss.U32(block, 4)
	for i := 0; i < 16; i++ {
		ci := (bits >> (uint(i) * 2)) & 3
		a := uint8(255)
		if transparent && ci == 3 {
			a = 0
		}
		setPixel(img, ox+i%4, oy+i/4, pal[ci][0], pal[ci][1], pal[ci][2], a)
	}
}

func decodeBC3Block(block []byte, img *image.NRGBA, ox, oy int) {
	// alpha (BC4): two endpoints + 16 x 3-bit indices (48 bits)
	a0, a1 := block[0], block[1]
	var alpha [8]uint8
	alpha[0], alpha[1] = a0, a1
	if a0 > a1 {
		for i := 1; i <= 6; i++ {
			alpha[i+1] = uint8((int(6-i)*int(a0) + int(i)*int(a1)) / 6)
		}
	} else {
		for i := 1; i <= 4; i++ {
			alpha[i+1] = uint8((int(4-i)*int(a0) + int(i)*int(a1)) / 4)
		}
		alpha[6], alpha[7] = 0, 255
	}
	aBits := uint64(block[2]) | uint64(block[3])<<8 | uint64(block[4])<<16 |
		uint64(block[5])<<24 | uint64(block[6])<<32 | uint64(block[7])<<40

	pal := colorPalette(block[8:], true)
	cBits := bss.U32(block, 12)
	for i := 0; i < 16; i++ {
		ci := (cBits >> (uint(i) * 2)) & 3
		ai := (aBits >> (uint(i) * 3)) & 7
		setPixel(img, ox+i%4, oy+i/4, pal[ci][0], pal[ci][1], pal[ci][2], alpha[ai])
	}
}

func setPixel(img *image.NRGBA, x, y int, r, g, b, a uint8) {
	if x >= img.Rect.Max.X || y >= img.Rect.Max.Y {
		return
	}
	o := img.PixOffset(x, y)
	img.Pix[o+0], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = r, g, b, a
}
