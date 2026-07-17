package bss

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

func TestDecodeUTF16(t *testing.T) {
	tests := []struct {
		name  string
		units []uint16
		odd   bool
	}{
		{name: "empty"},
		{name: "ascii", units: []uint16{'B', 'D', 'O'}},
		{name: "korean", units: []uint16{'검', '은', '사', '막'}},
		{name: "surrogate pair", units: []uint16{0xD83D, 0xDE00}},
		{name: "unpaired high", units: []uint16{0xD800, 'x'}},
		{name: "unpaired low", units: []uint16{0xDC00}},
		{name: "odd trailing byte", units: []uint16{'x'}, odd: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := make([]byte, len(test.units)*2, len(test.units)*2+1)
			for i, unit := range test.units {
				binary.LittleEndian.PutUint16(raw[i*2:], unit)
			}
			if test.odd {
				raw = append(raw, 0xff)
			}
			want := string(utf16.Decode(test.units))
			if got := DecodeUTF16(raw); got != want {
				t.Fatalf("DecodeUTF16() = %q, want %q", got, want)
			}
		})
	}
}
