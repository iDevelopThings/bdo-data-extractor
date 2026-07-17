package urn

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const prefix = "urn::"

var (
	ErrInvalid = errors.New("invalid urn")
)

type URN struct {
	Domain string
	Kind   string
	ID     string
}

func (u URN) String() string {
	if u.Kind == "" {
		return prefix + u.Domain + ":" + u.ID
	}

	return prefix + u.Domain + ":" + u.Kind + ":" + u.ID
}

func (u URN) Uint32() (uint32, error) {
	n, err := strconv.ParseUint(u.ID, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%w: id %q is not uint32", ErrInvalid, u.ID)
	}

	return uint32(n), nil
}
func (u URN) Uint32Unsafe() uint32 {
	n, err := strconv.ParseUint(u.ID, 10, 32)
	if err != nil {
		panic(fmt.Sprintf("invalid urn: id %q is not uint32", u.ID))
	}

	return uint32(n)
}

// MarshalText encodes the URN as its "urn::domain:kind:id" string. It is a
// TextMarshaler (not a json.Marshaler) deliberately: encoding/json still wraps
// the text in a JSON string, but Wails' TS binding generator types a
// TextMarshaler as `string` rather than the opaque `any` it emits for
// json.Marshaler — so the desktop app's bindings get a real string type.
func (u URN) MarshalText() ([]byte, error) {
	return []byte(u.String()), nil
}

func (u *URN) UnmarshalText(text []byte) error {
	parsed, err := Parse(string(text))
	if err != nil {
		return err
	}

	*u = parsed
	return nil
}

func (u URN) IsValid() bool {
	return u.Domain != ""
}

func Parse(raw string) (URN, error) {
	if !strings.HasPrefix(raw, prefix) {
		return URN{}, fmt.Errorf("%w: %q does not start with %q", ErrInvalid, raw, prefix)
	}

	parts := strings.Split(strings.TrimPrefix(raw, prefix), ":")
	if len(parts) != 2 && len(parts) != 3 {
		return URN{}, fmt.Errorf("%w: expected 2 or 3 parts, got %d", ErrInvalid, len(parts))
	}
	for _, part := range parts {
		if part == "" {
			return URN{}, fmt.Errorf("%w: empty part in %q", ErrInvalid, raw)
		}
	}

	if len(parts) == 2 {
		return URN{Domain: parts[0], ID: parts[1]}, nil
	}

	return URN{Domain: parts[0], Kind: parts[1], ID: parts[2]}, nil
}

var (
	Item        = NewHandler("item").EnsureRegistered()
	ItemSet     = NewHandler("item-set").EnsureRegistered()
	Enhancement = NewHandler("enhancement").EnsureRegistered()
	NPC         = NewHandler("npc").EnsureRegistered()
	GrindSpot   = NewHandler("grindspot").EnsureRegistered()
	Character   = NewHandler("character").EnsureRegistered()
	Caphras     = NewHandler("caphras").EnsureRegistered()
	Knowledge   = NewHandler("knowledge").Kinds("theme", "entry").EnsureRegistered()
	World       = NewHandler("world").Kinds("region", "node", "territory").EnsureRegistered()
	// Recipe URNs are urn::recipe:<outputItemId>:<index> — the kind is the output
	// item id (dynamic) and the id is the per-output recipe index.
	Recipe = NewHandler("recipe").DynamicKinds().EnsureRegistered()
)
