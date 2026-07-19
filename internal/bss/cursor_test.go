package bss

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestCursorZero(t *testing.T) {
	c := NewCursor([]byte{0, 0, 1, 0}, 0, 4)
	if !c.Zero(2) || c.Pos() != 2 {
		t.Fatalf("zero prefix: ok=%v pos=%d", c.OK(), c.Pos())
	}
	if c.Zero(1) || !c.OK() || c.Pos() != 3 {
		t.Fatalf("nonzero byte: ok=%v pos=%d", c.OK(), c.Pos())
	}
	if c.Zero(2) || c.OK() {
		t.Fatalf("past end: ok=%v pos=%d", c.OK(), c.Pos())
	}
}

func TestCursorRepeated(t *testing.T) {
	c := NewCursor([]byte{0x2e, 0x2e, 0x2e, 1}, 0, 4)
	if !c.Repeated(3, 0x2e) {
		t.Fatal("Repeated rejected a matching run")
	}
	if c.Pos() != 3 {
		t.Fatalf("position = %d, want 3", c.Pos())
	}
	if c.Repeated(1, 0x2e) {
		t.Fatal("Repeated accepted a mismatching byte")
	}
	if c.Pos() != 4 {
		t.Fatalf("position = %d, want 4", c.Pos())
	}
}

func TestAllZero(t *testing.T) {
	if !AllZero(nil) || !AllZero([]byte{0, 0}) || AllZero([]byte{0, 1}) {
		t.Fatal("unexpected AllZero result")
	}
}

func TestCursorRejectsOverflowingLengths(t *testing.T) {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, math.MaxUint64)
	c := NewCursor(data, 0, len(data))
	if value := c.UTF16(); value != "" || c.OK() {
		t.Fatalf("UTF16() = %q, ok=%v", value, c.OK())
	}

	c = NewCursor([]byte{1}, 0, 1)
	if value := c.Bytes(-1); value != nil || c.OK() {
		t.Fatalf("Bytes(-1) = %v, ok=%v", value, c.OK())
	}
}

func TestIndexInlineUTF16ByEnd(t *testing.T) {
	t.Parallel()

	data := make([]byte, 3)
	start := len(data)
	b := EncodeUTF16("검은사막")
	data = binary.LittleEndian.AppendUint64(data, uint64(len([]rune("검은사막"))))
	data = append(data, b...)
	end := len(data)
	data = append(data, make([]byte, 8)...)

	got := IndexInlineUTF16ByEnd(data)
	if len(got[end]) != 1 || got[end][0] != start {
		t.Fatalf("starts at end %d = %v, want [%d]", end, got[end], start)
	}
	if _, ok := got[len(data)]; ok {
		t.Fatal("empty string was indexed")
	}
}
