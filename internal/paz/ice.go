// Package paz implements the read-only Black Desert Online PAZ archive layer:
// the ICE cipher, the custom LZ decompressor, the pad00000.meta index parser,
// and targeted file extraction.
package paz

import "encoding/binary"

// BDOICEKey is BDO's thin-ICE key (white-desert lib.rs BDO_ICE_KEY).
var BDOICEKey = [8]byte{0x51, 0xF3, 0x0F, 0x11, 0x04, 0x24, 0x6A, 0x00}

var iceSMod = [4][4]uint32{
	{333, 313, 505, 369}, {379, 375, 319, 391},
	{361, 445, 451, 397}, {397, 425, 395, 505},
}
var iceSXor = [4][4]uint32{
	{0x83, 0x85, 0x9b, 0xcd}, {0xcc, 0xa7, 0xad, 0x41},
	{0x4b, 0x2e, 0xd4, 0x33}, {0xea, 0xcb, 0x2e, 0x04},
}
var icePBox = [32]uint32{
	0x00000001, 0x00000080, 0x00000400, 0x00002000, 0x00080000, 0x00200000, 0x01000000, 0x40000000,
	0x00000008, 0x00000020, 0x00000100, 0x00004000, 0x00010000, 0x00800000, 0x04000000, 0x20000000,
	0x00000004, 0x00000010, 0x00000200, 0x00008000, 0x00020000, 0x00400000, 0x08000000, 0x10000000,
	0x00000002, 0x00000040, 0x00000800, 0x00001000, 0x00040000, 0x00100000, 0x02000000, 0x80000000,
}
var keyRot = [16]int{0, 1, 2, 3, 2, 1, 3, 0, 1, 3, 2, 0, 3, 1, 0, 2}

func gfMult(a, b, m uint32) uint32 {
	var res uint32
	for b != 0 {
		if b&1 != 0 {
			res ^= a
		}
		a <<= 1
		b >>= 1
		if a >= 256 {
			a ^= m
		}
	}
	return res
}

func gfExp7(b, m uint32) uint32 {
	if b == 0 {
		return 0
	}
	x := gfMult(b, b, m)
	x = gfMult(b, x, m)
	x = gfMult(x, x, m)
	return gfMult(b, x, m)
}

func icePerm32(x uint32) uint32 {
	var res uint32
	for _, pb := range icePBox {
		if x&1 != 0 {
			res |= pb
		}
		x >>= 1
	}
	return res
}

func buildSbox() []uint32 {
	sbox := make([]uint32, 4096)
	for i := 0; i < 1024; i++ {
		col := uint32((i >> 1) & 0xFF)
		row := (i & 0x1) | ((i & 0x200) >> 8)
		sbox[i] = icePerm32(gfExp7(col^iceSXor[0][row], iceSMod[0][row]) << 24)
		sbox[1024+i] = icePerm32(gfExp7(col^iceSXor[1][row], iceSMod[1][row]) << 16)
		sbox[2048+i] = icePerm32(gfExp7(col^iceSXor[2][row], iceSMod[2][row]) << 8)
		sbox[3072+i] = icePerm32(gfExp7(col^iceSXor[3][row], iceSMod[3][row]))
	}
	return sbox
}

var iceSbox = buildSbox()

func rotl(v uint32, n uint) uint32 { return (v << n) | (v >> (32 - n)) }

// ICE is a thin-ICE (level 0) cipher instance: 8-byte key, 8 rounds.
type ICE struct {
	rounds   int
	keysched [][3]uint32
}

// NewICE builds an ICE instance for the given 8-byte key.
func NewICE(key [8]byte) *ICE {
	c := &ICE{rounds: 8, keysched: make([][3]uint32, 8)}
	c.keySet(key)
	return c
}

func (c *ICE) keySchedBuild(kb *[4]uint32, n int) {
	for i := 0; i < 8; i++ {
		kr := keyRot[i]
		isk := &c.keysched[n+i]
		isk[0], isk[1], isk[2] = 0, 0, 0
		for s := 0; s < 5; s++ {
			for j := 0; j < 3; j++ {
				cur := isk[j]
				for k := 0; k < 4; k++ {
					idx := (kr + k) & 3
					bit := kb[idx] & 1
					cur = (cur << 1) | bit
					kb[idx] = ((kb[idx] >> 1) | ((bit ^ 1) << 15)) & 0xFFFF
				}
				isk[j] = cur
			}
		}
	}
}

func (c *ICE) keySet(key [8]byte) {
	var kb [4]uint32
	for i := 0; i < 4; i++ {
		kb[3-i] = (uint32(key[i*2]) << 8) | uint32(key[i*2+1])
	}
	c.keySchedBuild(&kb, 0)
}

func iceF(p uint32, sk *[3]uint32) uint32 {
	tr := (p & 0x3FF) | ((p << 2) & 0xFFC00)
	tl := ((p >> 16) & 0x3FF) | (rotl(p, 18) & 0xFFC00)
	salt := sk[2] & (tl ^ tr)
	al := salt ^ tl ^ sk[0]
	ar := salt ^ tr ^ sk[1]
	return iceSbox[(al>>10)&0x3FF] ^
		iceSbox[1024+(al&0x3FF)] ^
		iceSbox[2048+((ar>>10)&0x3FF)] ^
		iceSbox[3072+(ar&0x3FF)]
}

func (c *ICE) cryptBlock(l, r uint32, encrypt bool) (uint32, uint32) {
	ks := c.keysched
	if encrypt {
		for i := 0; i < c.rounds; i += 2 {
			l ^= iceF(r, &ks[i])
			r ^= iceF(l, &ks[i+1])
		}
	} else {
		for i := c.rounds - 2; i >= 0; i -= 2 {
			l ^= iceF(r, &ks[i+1])
			r ^= iceF(l, &ks[i])
		}
	}
	return l, r
}

func (c *ICE) crypt(data []byte, encrypt bool) []byte {
	n := len(data) - (len(data) % 8)
	for off := 0; off < n; off += 8 {
		l := binary.BigEndian.Uint32(data[off : off+4])
		r := binary.BigEndian.Uint32(data[off+4 : off+8])
		l, r = c.cryptBlock(l, r, encrypt)
		binary.BigEndian.PutUint32(data[off:off+4], r) // halves swapped on write-back
		binary.BigEndian.PutUint32(data[off+4:off+8], l)
	}
	return data
}

// Decrypt decrypts whole 64-bit blocks in place and returns the slice.
func (c *ICE) Decrypt(data []byte) []byte { return c.crypt(data, false) }

// Encrypt encrypts whole 64-bit blocks in place and returns the slice.
func (c *ICE) Encrypt(data []byte) []byte { return c.crypt(data, true) }
