package build

import (
	"fmt"

	"github.com/dgravesa/go-parallel/parallel"
)

// readFiles loads each basename from the PAZ source. Multiple names are read
// concurrently (Archive.Content and Index.Find are concurrent-safe).
func (b *Builder) readFiles(names ...string) ([][]byte, error) {
	out := make([][]byte, len(names))
	errs := make([]error, len(names))
	switch len(names) {
	case 0:
		return out, nil
	case 1:
		out[0], errs[0] = b.src.Read(names[0])
	default:
		parallel.For(len(names), func(i, _ int) {
			out[i], errs[i] = b.src.Read(names[i])
		})
	}
	for i, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("%s: %w", names[i], err)
		}
	}
	return out, nil
}
