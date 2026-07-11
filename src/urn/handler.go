package urn

import (
	"fmt"
	"strings"
)

func WithDomain(domain string) func(Handler) Handler {
	return func(h Handler) Handler {
		h.domain = strings.ToLower(domain)
		return h
	}
}

func WithKinds(kinds ...string) func(Handler) Handler {
	return func(h Handler) Handler {
		h.kinds = make(map[string]struct{}, len(kinds))
		for _, kind := range kinds {
			h.kinds[kind] = struct{}{}
		}

		return h
	}
}

type Handler struct {
	domain  string
	kinds   map[string]struct{}
	dynamic bool // accept any non-empty kind (e.g. recipe's numeric output key)
}

func NewHandler(domain string) Handler {
	return Handler{domain: strings.ToLower(domain)}
}
func NewHandlerWithOptions(options ...func(Handler) Handler) Handler {
	h := Handler{}
	for _, option := range options {
		h = option(h)
	}

	if h.domain == "" {
		panic("domain must be set")
	}

	return h
}

func (h Handler) Kinds(kinds ...string) Handler {
	next := Handler{
		domain: h.domain,
		kinds:  make(map[string]struct{}, len(kinds)),
	}
	for _, kind := range kinds {
		next.kinds[kind] = struct{}{}
	}

	return next
}

// DynamicKinds returns a handler that accepts any non-empty kind rather than a
// fixed set — for domains whose kind is a data value, e.g. recipe URNs keyed by
// their output item id (urn::recipe:<outputId>:<index>).
func (h Handler) DynamicKinds() Handler {
	return Handler{domain: h.domain, dynamic: true}
}

func (h Handler) Domain() string {
	return h.domain
}

func (h Handler) KindList() []string {
	out := make([]string, 0, len(h.kinds))
	for kind := range h.kinds {
		out = append(out, kind)
	}

	return out
}

func (h Handler) New(parts ...any) URN {
	urn, err := h.ParseParts(parts...)
	if err != nil {
		panic(err)
	}

	return urn
}

func (h Handler) ParseParts(parts ...any) (URN, error) {
	switch {
	case h.hasKinds() && len(parts) == 2:
		kind := fmt.Sprint(parts[0])
		if !h.validKind(kind) {
			return URN{}, fmt.Errorf("%w: %q is not a valid %s kind", ErrInvalid, kind, h.domain)
		}

		return URN{Domain: h.domain, Kind: kind, ID: fmt.Sprint(parts[1])}, nil
	case !h.hasKinds() && len(parts) == 1:
		return URN{Domain: h.domain, ID: fmt.Sprint(parts[0])}, nil
	default:
		return URN{}, fmt.Errorf("%w: %s expects %d part(s), got %d", ErrInvalid, h.domain, h.expectedParts(), len(parts))
	}
}

func (h Handler) Parse(raw string) (URN, error) {
	u, err := Parse(raw)
	if err != nil {
		return URN{}, err
	}
	if u.Domain != h.domain {
		return URN{}, fmt.Errorf("%w: expected domain %q, got %q", ErrInvalid, h.domain, u.Domain)
	}
	if h.hasKinds() && !h.validKind(u.Kind) {
		return URN{}, fmt.Errorf("%w: %q is not a valid %s kind", ErrInvalid, u.Kind, h.domain)
	}
	if !h.hasKinds() && u.Kind != "" {
		return URN{}, fmt.Errorf("%w: %s does not accept kind %q", ErrInvalid, h.domain, u.Kind)
	}

	return u, nil
}

func (h Handler) Match(u URN, kind ...string) bool {
	if u.Domain != h.domain {
		return false
	}
	if len(kind) > 0 {
		return u.Kind == kind[0]
	}
	if h.hasKinds() {
		return h.validKind(u.Kind)
	}

	return u.Kind == ""
}

func (h Handler) hasKinds() bool {
	return h.dynamic || len(h.kinds) > 0
}

func (h Handler) validKind(kind string) bool {
	if h.dynamic {
		return kind != ""
	}
	_, ok := h.kinds[kind]
	return ok
}

func (h Handler) expectedParts() int {
	if h.hasKinds() {
		return 2
	}

	return 1
}

func (h Handler) EnsureRegistered() Handler {
	RegisterHandlerUntyped(h)
	return h
}
