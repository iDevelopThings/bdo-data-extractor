package paz

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/idevelopthings/bdo-data-extractor/src/utils"
)

// Source bundles the parsed archive index with read access to the archive. It
// centralizes the load-meta / open / find / read / close sequence the commands
// share. Call Close when done. Not safe for concurrent use.
type Source struct {
	Index   *Index
	Archive *Archive
}

var (
	isOpen     atomic.Bool
	openSource *Source
)

// OpenSource loads pad00000.meta and opens the archive (read-only).
func OpenSource(gameDir string) (*Source, error) {
	if isOpen.Load() {
		return openSource, nil
	}

	ix, err := LoadMeta(gameDir)
	if err != nil {
		return nil, fmt.Errorf("load meta: %w", err)
	}
	s := &Source{Index: ix, Archive: Open(gameDir)}

	isOpen.Store(true)
	openSource = s

	return s, nil
}

// Read returns the decoded bytes of the file whose basename is name.
func (s *Source) Read(name string) ([]byte, error) {
	timed := utils.Timed(fmt.Sprintf("paz.Read[%q]", name))
	defer timed()

	b, exists, err := s.read(name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("table not found: %s", name)
	}

	return b, nil
}

// ReadIfExists returns decoded file bytes and whether the basename exists.
func (s *Source) ReadIfExists(name string) ([]byte, bool, error) {
	timed := utils.Timed(fmt.Sprintf("paz.ReadIfExists[%q]", name))
	defer timed()

	return s.read(name)
}

func (s *Source) read(name string) ([]byte, bool, error) {
	f, ok := s.Index.Find(name)
	if !ok {
		return nil, false, nil
	}
	b, err := s.Archive.Content(f)
	if err != nil {
		return nil, true, err
	}
	if f.CompSize == f.OrigSize && strings.EqualFold(filepath.Ext(name), ".bss") {
		b = decodeInnerPABR(b)
	}

	return b, true, nil
}

// ReadAny returns the decoded bytes of the first of names that exists, and which
// one matched.
func (s *Source) ReadAny(names ...string) ([]byte, string, error) {
	for _, name := range names {
		if b, ok, err := s.read(name); ok {
			return b, name, err
		}
	}
	return nil, "", fmt.Errorf("none found: %v", names)
}

// Close releases the archive's open file handles.
func (s *Source) Close() {
	s.Archive.Close()

	if openSource == s {
		openSource = nil
		isOpen.Store(false)
	}
}
