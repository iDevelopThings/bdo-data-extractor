package loc

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

func TestDecodeLuaStringsAndResolve(t *testing.T) {
	data := make([]byte, 8)
	copy(data, "PABR")
	binary.LittleEndian.PutUint32(data[4:], 1)

	data = binary.LittleEndian.AppendUint32(data, 0xbdc20727)
	data = binary.LittleEndian.AppendUint32(data, 0)
	data = binary.LittleEndian.AppendUint32(data, 2)
	for _, entry := range [][3]uint32{{100, 1, 2}, {200, 3, 4}} {
		data = binary.LittleEndian.AppendUint32(data, entry[0])
		data = binary.LittleEndian.AppendUint32(data, entry[1])
		data = binary.LittleEndian.AppendUint32(data, entry[2])
		data = binary.LittleEndian.AppendUint32(data, 0)
	}

	stringTablePos := len(data)
	strings := []string{"GAME", "LUA_FIRST", "첫 번째", "LUA_SECOND", "두 번째"}
	data = binary.LittleEndian.AppendUint32(data, uint32(len(strings)))
	data = append(data, 1)
	for _, value := range strings {
		encoded := encodeUTF16(value)
		data = binary.LittleEndian.AppendUint32(data, uint32(len(encoded)))
		data = append(data, encoded...)
		data = append(data, 1)
	}
	data = binary.LittleEndian.AppendUint64(data, uint64(stringTablePos))

	catalog, err := DecodeLuaStrings(data)
	if err != nil {
		t.Fatal(err)
	}
	catalog.Resolve(Table{
		100: {1: "First"},
		200: {0x10001: "Second"},
	})

	first, ok := catalog.Lookup("GAME", "LUA_FIRST")
	if !ok || first.Text != "First" || first.Field != 1 || first.SourceFallback {
		t.Fatalf("first = %+v, found=%v", first, ok)
	}
	second, ok := catalog.Lookup("GAME", "LUA_SECOND")
	if !ok || second.Text != "Second" || second.Field != 0x10001 || second.SourceFallback {
		t.Fatalf("second = %+v, found=%v", second, ok)
	}
}

func encodeUTF16(value string) []byte {
	units := utf16.Encode([]rune(value))
	out := make([]byte, len(units)*2)
	for i, unit := range units {
		binary.LittleEndian.PutUint16(out[i*2:], unit)
	}
	return out
}
