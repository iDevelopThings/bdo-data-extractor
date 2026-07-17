package main

import (
	"strings"
	"unicode"
)

// pascal converts a delimited or already-cased identifier to PascalCase,
// splitting on '_', '-', ' ', and '.'.
func pascal(s string) string {
	var b strings.Builder
	up := true
	for _, r := range s {
		switch {
		case r == '_' || r == '-' || r == ' ' || r == '.':
			up = true
		case up:
			b.WriteRune(unicode.ToUpper(r))
			up = false
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// camel converts an identifier to camelCase (PascalCase with a lowercased head).
func camel(s string) string {
	return unexport(pascal(s))
}

// unexport lowercases the first rune, yielding an unexported Go identifier.
func unexport(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

// snake converts a PascalCase identifier to snake_case.
func snake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
