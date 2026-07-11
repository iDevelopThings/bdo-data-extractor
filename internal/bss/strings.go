package bss

// The tables store inline strings as a recurring shape: a u32 byte-length
// followed by that many bytes of little-endian UTF-16. Names, descriptions, icon
// paths and PABR string tables all use it. These helpers decode it in one place
// so callers stop re-deriving the layout by hand.

// ReadLenUTF16 reads a length-prefixed UTF-16LE string at pos —
// [u32 byteLen][byteLen bytes of UTF-16] — and returns the decoded string and
// the offset just past it. On a short/out-of-range read it returns ("", pos).
func ReadLenUTF16(b []byte, pos int) (string, int) {
	if pos < 0 || pos+4 > len(b) {
		return "", pos
	}
	n := int(U32(b, pos))
	pos += 4
	if n < 0 || pos+n > len(b) {
		return "", pos
	}
	return DecodeUTF16(b[pos : pos+n]), pos + n
}

// PeekUTF16Chars reports the character count of a plausible item-record inline
// string at pos — [i64 charCount][charCount×2 bytes UTF-16LE] — validated by
// sampling the leading chars for Hangul or printable ASCII, or 0 if pos doesn't
// look like one. Used by sequential decoders to tell a length-prefixed string
// apart from a run of scalars without a schema. (The item record uses an i64
// char count, unlike ReadLenUTF16's u32 byte length.)
func PeekUTF16Chars(b []byte, pos int) int {
	if pos < 0 || pos+8 > len(b) {
		return 0
	}
	n := int(int64(U64(b, pos)))
	if n < 3 || n > 3000 || pos+8+n*2 > len(b) {
		return 0
	}
	chk := n
	if chk > 20 {
		chk = 20
	}
	good := 0
	for k := 0; k < chk; k++ {
		u := uint16(b[pos+8+k*2]) | uint16(b[pos+8+k*2+1])<<8
		if (u >= 0xAC00 && u <= 0xD7A3) || (u >= 32 && u < 127) {
			good++
		}
	}
	if good >= chk*8/10 {
		return n
	}
	return 0
}

// A PABR string table is [u32 count][u8 sep] then count × ( [u32 byteLen][bytes]
// [u8 sep] ), the byte between entries being a 0x01 record separator. The entry
// bytes are raw (UTF-8/ASCII) in some tables and UTF-16 in others; the two public
// readers below pick the decoding.

func readStringTable(b []byte, pos int, decode func([]byte) string) []string {
	if pos < 0 || pos+4 > len(b) {
		return nil
	}
	count := int(U32(b, pos))
	pos += 4
	if count <= 0 || count > len(b) {
		return nil
	}
	pos++ // leading separator
	out := make([]string, 0, count)
	for i := 0; i < count && pos+4 <= len(b); i++ {
		n := int(U32(b, pos))
		pos += 4
		if n < 0 || pos+n > len(b) {
			break
		}
		out = append(out, decode(b[pos:pos+n]))
		pos += n + 1 // entry bytes + trailing separator
	}
	return out
}

// ReadStringTable parses a PABR string table whose entries are raw bytes
// (UTF-8/ASCII).
func ReadStringTable(b []byte, pos int) []string {
	return readStringTable(b, pos, func(entry []byte) string { return string(entry) })
}

// ReadUTF16StringTable parses a PABR string table whose entries are UTF-16LE.
func ReadUTF16StringTable(b []byte, pos int) []string {
	return readStringTable(b, pos, DecodeUTF16)
}
