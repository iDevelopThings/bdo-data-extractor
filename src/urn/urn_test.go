package urn

import (
	"encoding/json"
	"testing"
)

func TestHandlerBuildsURNs(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "plain domain", got: Item.New(uint32(12345)).String(), want: "urn::item:12345"},
		{name: "kinded domain", got: Knowledge.New("entry", 90031).String(), want: "urn::knowledge:entry:90031"},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				if tt.got != tt.want {
					t.Fatalf("got %q, want %q", tt.got, tt.want)
				}
			},
		)
	}
}

func TestParse(t *testing.T) {
	u, err := Parse("urn::world:region:310")
	if err != nil {
		t.Fatal(err)
	}
	if u.Domain != "world" || u.Kind != "region" || u.ID != "310" {
		t.Fatalf("unexpected urn: %#v", u)
	}
}

func TestHandlerParseRejectsWrongKind(t *testing.T) {
	if _, err := Knowledge.Parse("urn::knowledge:node:123"); err == nil {
		t.Fatal("expected invalid kind error")
	}
}

func TestJSONUsesStringForm(t *testing.T) {
	data, err := json.Marshal(Item.New(12345))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `"urn::item:12345"` {
		t.Fatalf("got %s", data)
	}

	var u URN
	if err := json.Unmarshal(data, &u); err != nil {
		t.Fatal(err)
	}
	if !Item.Match(u) || u.ID != "12345" {
		t.Fatalf("unexpected urn: %#v", u)
	}
}

func TestUint32(t *testing.T) {
	id, err := Item.New(12345).Uint32()
	if err != nil {
		t.Fatal(err)
	}
	if id != 12345 {
		t.Fatalf("got %d, want 12345", id)
	}
}

func TestTyped(t *testing.T) {

	type TestingType struct{}

	RegisterTypedHandler[TestingType](
		WithKinds("testing", "fdsfs"),
	)

	h, ok := GetHandlerByType[TestingType]()
	if !ok {
		t.Fatal("expected to find handler for TestingType")
	}
	if h.Domain() != "testingtype" {
		t.Fatalf("expected domain 'testingtype', got %q", h.Domain())
	}

}
