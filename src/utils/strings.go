package utils

import (
	"strings"
	"unicode"
)

// IconExt is the extension the icon pipeline writes decoded art as. The table
// decoders name icon files before the pipeline writes them, so both sides derive
// the name from here instead of hardcoding an extension and drifting apart.
const IconExt = ".webp"

// IconFileName swaps a .dds asset path's extension for the icon pipeline's output
// extension: "ui_artwork/ic_09996.dds" -> "ui_artwork/ic_09996.webp".
func IconFileName(ddsPath string) string {
	return strings.TrimSuffix(ddsPath, ".dds") + IconExt
}

// Slug normalizes a display name into a stable, URN-safe id part: lowercased,
// every run of non-alphanumeric characters collapsed to a single underscore,
// leading/trailing underscores trimmed ("Kzarka Statue" → "kzarka_statue",
// "Altar of Blood - The 11th Illusion" → "altar_of_blood_the_11th_illusion").
// It never contains ':' so it is safe as a URN id, and is deterministic so the
// same name always yields the same slug.
func Slug(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevUnderscore := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
			prevUnderscore = false
		case !prevUnderscore:
			b.WriteByte('_')
			prevUnderscore = true
		}
	}

	return strings.Trim(b.String(), "_")
}

// HasKorean reports whether s contains any Hangul (Korean) character — used to
// tell Korean source strings apart from already-localized English ones.
func HasKorean(s string) bool {
	for _, r := range s {
		switch {
		case r >= 0xAC00 && r <= 0xD7A3, // Hangul syllables
			r >= 0x1100 && r <= 0x11FF, // Hangul Jamo
			r >= 0x3130 && r <= 0x318F: // Hangul compatibility Jamo
			return true
		}
	}

	return false
}

// HumanizeString turns "SOME_FUNC_NAME" into "Some Func Name".
func HumanizeString(fn string) string {
	words := strings.Fields(strings.ReplaceAll(strings.ToLower(fn), "_", " "))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
