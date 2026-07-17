package bss

import (
	"encoding/binary"
	"math"
)

// Standalone little-endian readers for random access into a decoded table — the
// recurring `binary.LittleEndian.Uint32(b[o:])` pattern, named once.
func U16(b []byte, o int) uint16  { return binary.LittleEndian.Uint16(b[o:]) }
func U32(b []byte, o int) uint32  { return binary.LittleEndian.Uint32(b[o:]) }
func U64(b []byte, o int) uint64  { return binary.LittleEndian.Uint64(b[o:]) }
func F32(b []byte, o int) float64 { return float64(math.Float32frombits(U32(b, o))) }

// AllZero reports whether every byte in b is zero.
func AllZero(b []byte) bool {
	for _, value := range b {
		if value != 0 {
			return false
		}
	}
	return true
}

// Cursor is a bounds-checked, byte-granular sequential reader over a (sub)slice
// of a decoded table. Records in these tables are byte-packed, so fields land at
// arbitrary offsets; the cursor advances by the exact field width. A read past
// the end sets a sticky error — check OK after a run of reads rather than each
// one. It is the byte-level counterpart to Reader (which is schema-driven).
type Cursor struct {
	b        []byte
	pos, end int
	bad      bool
}

// NewCursor reads b over [start, end). end is clamped to len(b).
func NewCursor(b []byte, start, end int) *Cursor {
	if end > len(b) {
		end = len(b)
	}
	return &Cursor{b: b, pos: start, end: end}
}

func (c *Cursor) OK() bool { return !c.bad }
func (c *Cursor) Pos() int { return c.pos }

func (c *Cursor) avail(n int) bool {
	if c.bad || c.pos+n > c.end {
		c.bad = true
		return false
	}
	return true
}

func (c *Cursor) U8() int {
	if !c.avail(1) {
		return 0
	}
	v := int(c.b[c.pos])
	c.pos++
	return v
}
func (c *Cursor) U16() uint32 {
	if !c.avail(2) {
		return 0
	}
	v := uint32(U16(c.b, c.pos))
	c.pos += 2
	return v
}
func (c *Cursor) U32() uint32 {
	if !c.avail(4) {
		return 0
	}
	v := U32(c.b, c.pos)
	c.pos += 4
	return v
}
func (c *Cursor) F32() float64 { return float64(math.Float32frombits(c.U32())) }

func (c *Cursor) U64() uint64 {
	if !c.avail(8) {
		return 0
	}
	v := U64(c.b, c.pos)
	c.pos += 8
	return v
}
func (c *Cursor) I16() int16   { return int16(c.U16()) }
func (c *Cursor) I32() int32   { return int32(c.U32()) }
func (c *Cursor) I64() int64   { return int64(c.U64()) }
func (c *Cursor) F64() float64 { return math.Float64frombits(c.U64()) }

// Byte reads one byte (Bool is the same read as a flag). U8 is kept for callers
// that want an int.
func (c *Cursor) Byte() byte { return byte(c.U8()) }
func (c *Cursor) Bool() bool { return c.U8() != 0 }

// Remaining is how many bytes are left before end (0 once errored).
func (c *Cursor) Remaining() int {
	if c.bad {
		return 0
	}
	return c.end - c.pos
}

// Skip advances n bytes (past-end sets the sticky error). Seek moves to an
// absolute offset. Both return the cursor for chaining.
func (c *Cursor) Skip(n int) *Cursor {
	if c.avail(n) {
		c.pos += n
	}
	return c
}
func (c *Cursor) Seek(pos int) *Cursor {
	if pos < 0 || pos > c.end {
		c.bad = true
		return c
	}
	c.pos = pos
	return c
}

// Bytes returns the next n bytes and advances; nil past the end.
func (c *Cursor) Bytes(n int) []byte {
	if !c.avail(n) {
		return nil
	}
	v := c.b[c.pos : c.pos+n]
	c.pos += n
	return v
}

// Zero consumes n bytes and reports whether they are all zero. A read past the
// cursor boundary returns false and sets the cursor's sticky error.
func (c *Cursor) Zero(n int) bool {
	b := c.Bytes(n)
	return c.OK() && AllZero(b)
}

// Repeated consumes n bytes and reports whether every byte equals value. A read
// past the cursor boundary returns false and sets the cursor's sticky error.
func (c *Cursor) Repeated(n int, value byte) bool {
	b := c.Bytes(n)
	if !c.OK() {
		return false
	}
	for _, current := range b {
		if current != value {
			return false
		}
	}
	return true
}

// PeekByte / PeekU32 read without advancing.
func (c *Cursor) PeekByte() byte {
	if c.bad || c.pos >= c.end {
		return 0
	}
	return c.b[c.pos]
}
func (c *Cursor) PeekU32() uint32 {
	if c.bad || c.pos+4 > c.end {
		return 0
	}
	return U32(c.b, c.pos)
}

// Fill consumes a run of the given byte (e.g. the 0x77 reserved-space filler)
// and returns how many it swallowed.
func (c *Cursor) Fill(b byte) int {
	n := 0
	for c.pos < c.end && c.b[c.pos] == b {
		c.pos++
		n++
	}
	return n
}

// UTF16 reads an inline item-record string: [i64 charCount][charCount×2 bytes of
// UTF-16LE]. (Distinct from ReadLenUTF16, which the PABR tables use with a u32
// byte-length.) UTF8 reads the [i64 byteCount][UTF-8] variant the icon path uses.
func (c *Cursor) UTF16() string {
	n := int(c.I64())
	if n < 0 || !c.avail(n*2) {
		c.bad = true
		return ""
	}
	s := DecodeUTF16(c.b[c.pos : c.pos+n*2])
	c.pos += n * 2
	return s
}
func (c *Cursor) UTF8() string {
	n := int(c.I64())
	if n < 0 || !c.avail(n) {
		c.bad = true
		return ""
	}
	s := string(c.b[c.pos : c.pos+n])
	c.pos += n
	return s
}

// U8N / U32N read a FIXED-size array (of n bytes / n little-endian u32s). Unlike
// U8List/U32List there is no length prefix — a C++ fixed array (e.g. the item
// slot list or skill-key slots) carries its size in the schema, not the data;
// unused entries hold a sentinel (46 = "none" for slots, 0 for keys). U8N copies
// into an owned slice (use Bytes for a non-copying view). A read past the end
// sets the sticky error. n <= 0 returns nil.
func (c *Cursor) U8N(n int) []byte {
	b := c.Bytes(n)
	if b == nil {
		return nil
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

func (c *Cursor) U32N(n int) []uint32 {
	if n <= 0 {
		return nil
	}
	out := make([]uint32, n)
	for i := 0; i < n; i++ {
		out[i] = c.U32()
	}
	return out
}

// U32List reads [u32 count][count×u32]; U16List reads u16 entries (widened to
// uint32). max guards against a misread count running away. A bad count sets the
// sticky error and returns nil.
func (c *Cursor) U32List(max int) []uint32 { return c.list(max, false) }
func (c *Cursor) U16List(max int) []uint32 { return c.list(max, true) }

func (c *Cursor) list(max int, u16 bool) []uint32 {
	n := int(c.U32())
	if n < 0 || n > max {
		c.bad = true
		return nil
	}
	out := make([]uint32, 0, n)
	for k := 0; k < n && c.OK(); k++ {
		if u16 {
			out = append(out, c.U16())
		} else {
			out = append(out, c.U32())
		}
	}
	return out
}
