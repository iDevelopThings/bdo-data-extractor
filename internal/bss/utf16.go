package bss

import (
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// DecodeUTF16 decodes raw little-endian UTF-16 bytes (for inline-string fields).
func DecodeUTF16(b []byte) string {
	utf8Len := 0
	for pos := 0; pos+1 < len(b); {
		r, next := decodeUTF16Rune(b, pos)
		utf8Len += utf8.RuneLen(r)
		pos = next
	}

	var out strings.Builder
	out.Grow(utf8Len)
	for pos := 0; pos+1 < len(b); {
		r, next := decodeUTF16Rune(b, pos)
		_, _ = out.WriteRune(r) // strings.Builder writes cannot fail
		pos = next
	}
	return out.String()
}

func decodeUTF16Rune(b []byte, pos int) (rune, int) {
	r := rune(U16(b, pos))
	next := pos + 2
	if r >= 0xD800 && r < 0xDC00 && next+1 < len(b) {
		low := rune(U16(b, next))
		if low >= 0xDC00 && low < 0xE000 {
			return utf16.DecodeRune(r, low), next + 2
		}
	}
	if r >= 0xD800 && r < 0xE000 {
		return unicode.ReplacementChar, next
	}
	return r, next
}
