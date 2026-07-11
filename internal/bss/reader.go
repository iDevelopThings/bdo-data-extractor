package bss

import (
	"fmt"
	"io"
	"math"
	"os"
	"unicode/utf16"
)

func debugEnabled() bool { return os.Getenv("BDO_DEBUG") != "" }

const pabrMagic = 0x52424150 // "PABR"

// Reader is a little-endian cursor over a decoded .bss/.dbss byte slice. Reads
// past the end set a sticky error (checked once after a record) rather than
// panicking, so a malformed/short record fails cleanly instead of crashing.
type Reader struct {
	b   []byte
	pos int
	err error
}

// NewReader wraps decoded table bytes.
func NewReader(b []byte) *Reader { return &Reader{b: b} }

// need reports whether n more bytes are available, setting the sticky error if not.
func (r *Reader) need(n int) bool {
	if r.err != nil {
		return false
	}
	if n < 0 || r.pos+n > len(r.b) {
		r.err = io.ErrUnexpectedEOF
		return false
	}
	return true
}

func (r *Reader) u8() byte {
	if !r.need(1) {
		return 0
	}
	v := r.b[r.pos]
	r.pos++
	return v
}
func (r *Reader) i16() int16 { return int16(r.u16()) }
func (r *Reader) u16() uint16 {
	if !r.need(2) {
		return 0
	}
	v := U16(r.b, r.pos)
	r.pos += 2
	return v
}
func (r *Reader) i32() int32 { return int32(r.u32()) }
func (r *Reader) u32() uint32 {
	if !r.need(4) {
		return 0
	}
	v := U32(r.b, r.pos)
	r.pos += 4
	return v
}
func (r *Reader) i64() int64 {
	if !r.need(8) {
		return 0
	}
	v := int64(U64(r.b, r.pos))
	r.pos += 8
	return v
}
func (r *Reader) f32() float32 { return math.Float32frombits(r.u32()) }

// take returns the next n bytes, or nil (and a sticky error) if they're not there.
func (r *Reader) take(n int) []byte {
	if !r.need(n) {
		return nil
	}
	b := r.b[r.pos : r.pos+n]
	r.pos += n
	return b
}

// fixedArray reads an inline length-prefixed blob: int64 count, then count*factor bytes.
func (r *Reader) fixedArray(factor int) []byte {
	n := int(r.i64())
	if r.err != nil || n < 0 || n > len(r.b) {
		r.err = io.ErrUnexpectedEOF
		return nil
	}
	return r.take(n * factor)
}

// readStringTable parses [int32 count][sep][ (int32 len)(bytes)(sep) ...].
func (r *Reader) readStringTable() []string {
	count := int(r.i32())
	if count <= 0 {
		return nil
	}
	r.u8() // leading separator
	out := make([]string, count)
	for i := 0; i < count; i++ {
		s := string(r.take(int(r.i32())))
		r.u8() // trailing separator
		if r.err != nil {
			return out
		}
		out[i] = s
	}
	return out
}

// ReadAll decodes every record of a table using the schema. PABR tables resolve
// string fields through the table's own string table; non-PABR tables hold inline
// strings. Returns the records decoded so far plus the first decode error.
func (r *Reader) ReadAll(s *Schema) ([]map[string]any, error) {
	if len(r.b) < 8 {
		return nil, fmt.Errorf("table too small")
	}
	magic := U32(r.b, 0)
	var strTable []string
	var rowCount int
	if magic == pabrMagic {
		stPos := int(U64(r.b, len(r.b)-8))
		if stPos < 0 || stPos >= len(r.b) {
			return nil, fmt.Errorf("bad string-table pointer %d", stPos)
		}
		r.pos = stPos
		strTable = r.readStringTable()
		rowCount = int(r.i32())
		if debugEnabled() {
			fmt.Printf(
				"[debug] stPos=%d strTableLen=%d rowCount=%d recordsStart=%d fileLen=%d\n",
				stPos, len(strTable), rowCount, r.pos, len(r.b),
			)
			for i := 0; i < len(strTable) && i < 6; i++ {
				fmt.Printf("[debug]   str[%d]=%q\n", i, strTable[i])
			}
		}
	} else {
		r.pos = 0
		rowCount = int(r.i32())
	}
	if r.err != nil {
		return nil, r.err
	}

	rows := make([]map[string]any, 0, rowCount)
	for i := 0; i < rowCount; i++ {
		row, err := r.readRecord(s, strTable)
		if err != nil {
			return rows, fmt.Errorf("record %d: %w", i, err)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (r *Reader) readRecord(s *Schema, strTable []string) (map[string]any, error) {
	m := make(map[string]any, len(s.Fields))
	for idx, f := range s.Fields {
		name := f.Name
		if name == "" || name == "Unk" {
			name = fmt.Sprintf("Unk%d", idx)
		}
		switch f.Type {
		case Text, UtfText, UniText:
			if strTable != nil {
				si := int(r.i32())
				if si >= 0 && si < len(strTable) {
					m[name] = strTable[si]
				} else {
					m[name] = ""
				}
				break
			}
			// inline string: int64 count, then count*factor bytes
			switch f.Type {
			case UtfText: // UTF-8, factor 1
				m[name] = string(r.fixedArray(1))
			case Text: // UTF-16, factor 2
				m[name] = DecodeUTF16(r.fixedArray(2))
			case UniText: // factor 4
				m[name] = DecodeUTF16(r.fixedArray(4))
			}
		case Byte:
			m[name] = r.u8()
		case Int16:
			m[name] = r.i16()
		case UInt16:
			m[name] = r.u16()
		case Int32:
			m[name] = r.i32()
		case UInt32:
			m[name] = r.u32()
		case Int64:
			m[name] = r.i64()
		case Float:
			m[name] = r.f32()
		case Bytes:
			m[name] = append([]byte(nil), r.take(f.Size)...)
		}
		if r.err != nil {
			return nil, fmt.Errorf("field %q: %w", name, r.err)
		}
	}
	return m, nil
}

// DecodeUTF16 decodes raw little-endian UTF-16 bytes (for inline-string fields).
func DecodeUTF16(b []byte) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = U16(b, i*2)
	}
	return string(utf16.Decode(u))
}
