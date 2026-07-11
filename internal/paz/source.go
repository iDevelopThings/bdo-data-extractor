package paz

import (
	"fmt"

	"github.com/idevelopthings/bdo-data-extractor/src/utils"
)

// Source bundles the parsed archive index with read access to the archive. It
// centralizes the load-meta / open / find / read / close sequence the commands
// share. Call Close when done. Not safe for concurrent use.
type Source struct {
	Index   *Index
	Archive *Archive
}

// OpenSource loads pad00000.meta and opens the archive (read-only).
func OpenSource(gameDir string) (*Source, error) {
	ix, err := LoadMeta(gameDir)
	if err != nil {
		return nil, fmt.Errorf("load meta: %w", err)
	}
	return &Source{Index: ix, Archive: Open(gameDir)}, nil
}

// Read returns the decoded bytes of the file whose basename is name.
func (s *Source) Read(name string) ([]byte, error) {
	timed := utils.Timed(fmt.Sprintf("paz.Read[%q]", name))
	defer timed()

	f, ok := s.Index.Find(name)
	if !ok {
		return nil, fmt.Errorf("table not found: %s", name)
	}
	return s.Archive.Content(f)
}

// ReadAny returns the decoded bytes of the first of names that exists, and which
// one matched.
func (s *Source) ReadAny(names ...string) ([]byte, string, error) {
	for _, name := range names {
		if f, ok := s.Index.Find(name); ok {
			b, err := s.Archive.Content(f)
			return b, name, err
		}
	}
	return nil, "", fmt.Errorf("none found: %v", names)
}

// Close releases the archive's open file handles.
func (s *Source) Close() { s.Archive.Close() }
