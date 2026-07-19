package bss

import "testing"

func TestPackPABRRoundTrip(t *testing.T) {
	t.Parallel()

	body := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	data := PackPABR(2, body)
	p, err := OpenPABR(data)
	if err != nil {
		t.Fatal(err)
	}
	if p.Rows != 2 || p.RecordsStart != 8 || p.StringTablePos != 8+len(body) {
		t.Fatalf("header = %+v", p)
	}
	got := data[p.RecordsStart:p.StringTablePos]
	if string(got) != string(body) {
		t.Fatalf("records = %v, want %v", got, body)
	}
}

func TestPABRCount(t *testing.T) {
	t.Parallel()

	if _, ok := PABRCount([]byte("nope")); ok {
		t.Fatal("expected false")
	}
	rows, ok := PABRCount([]byte{'P', 'A', 'B', 'R', 3, 0, 0, 0})
	if !ok || rows != 3 {
		t.Fatalf("rows=%d ok=%v", rows, ok)
	}
}
