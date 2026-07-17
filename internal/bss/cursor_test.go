package bss

import "testing"

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
