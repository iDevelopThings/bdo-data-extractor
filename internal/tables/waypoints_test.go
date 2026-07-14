package tables

import (
	"encoding/binary"
	"math"
	"testing"
	"unicode/utf16"
)

func TestDecodeWorldWaypoints(t *testing.T) {
	data := waypointFixture()
	waypoints, err := DecodeWorldWaypoints(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(waypoints) != 2 {
		t.Fatalf("got %d waypoints, want 2", len(waypoints))
	}
	first := waypoints[10]
	if first.Position != [3]float64{1.5, -2.5, 3.5} {
		t.Fatalf("position = %v", first.Position)
	}
	if first.InternalName != "town(first)" {
		t.Fatalf("internal name = %q", first.InternalName)
	}
	if first.Flags != [3]byte{16, 1, 0} {
		t.Fatalf("flags = %v", first.Flags)
	}
	if len(first.Links) != 1 || first.Links[0] != 20 {
		t.Fatalf("links = %v", first.Links)
	}
}

func waypointFixture() []byte {
	data := []byte{'P', 'A', 'B', 'R', 2, 0, 0, 0}
	appendU32 := func(v uint32) {
		data = binary.LittleEndian.AppendUint32(data, v)
	}
	appendF32 := func(v float32) {
		appendU32(math.Float32bits(v))
	}
	for i, row := range []struct {
		key   uint32
		pos   [3]float32
		flags [3]byte
	}{
		{10, [3]float32{1.5, -2.5, 3.5}, [3]byte{16, 1, 0}},
		{20, [3]float32{4.5, 5.5, 6.5}, [3]byte{}},
	} {
		appendU32(row.key)
		appendU32(uint32(i))
		for _, v := range row.pos {
			appendF32(v)
		}
		data = append(data, row.flags[:]...)
	}
	appendU32(1)
	appendU32(10)
	appendU32(20)
	data = append(data, 0, 0, 0, 0, 0)
	stringTablePos := len(data)
	appendU32(2)
	data = append(data, 0)
	for _, value := range []string{"town(first)", "field(second)"} {
		var raw []byte
		for _, unit := range utf16.Encode([]rune(value)) {
			raw = binary.LittleEndian.AppendUint16(raw, unit)
		}
		appendU32(uint32(len(raw)))
		data = append(data, raw...)
		data = append(data, 0)
	}
	data = binary.LittleEndian.AppendUint64(data, uint64(stringTablePos))

	return data
}
