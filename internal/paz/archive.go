package paz

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
)

// Archive provides read-only access to decoded file content from the PAZ set.
// It keeps each pad*.paz handle open across reads (Content is called thousands of
// times during a build); call Close when done. Content is safe for concurrent
// use: handle creation is mutex-guarded and reads use positional ReadAt.
type Archive struct {
	GameDir string
	ice     *ICE
	mu      sync.Mutex
	handles map[uint32]*os.File
}

// Open builds an Archive for a game directory (read-only).
func Open(gameDir string) *Archive {
	return &Archive{GameDir: gameDir, ice: NewICE(BDOICEKey), handles: map[uint32]*os.File{}}
}

// paz returns the (cached) open handle for volume n. Safe for concurrent use.
func (a *Archive) paz(n uint32) (*os.File, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if fh, ok := a.handles[n]; ok {
		return fh, nil
	}
	// The game ships these uppercase (PAD00001.PAZ) but the meta beside them
	// lowercase; the lowercase fallback is for installs that don't. Case only
	// matters on a case-sensitive filesystem — on Windows either name opens.
	fh, err := os.Open(filepath.Join(PazDir(a.GameDir), fmt.Sprintf("PAD%05d.PAZ", n))) // READ-ONLY
	if errors.Is(err, fs.ErrNotExist) {
		fh, err = os.Open(filepath.Join(PazDir(a.GameDir), fmt.Sprintf("pad%05d.paz", n))) // READ-ONLY
	}
	if err != nil {
		return nil, err
	}
	a.handles[n] = fh
	return fh, nil
}

// Close releases all cached .paz handles.
func (a *Archive) Close() {
	for _, fh := range a.handles {
		err := fh.Close()
		if err != nil {
			log.Printf("warning: failed to close %s: %v", fh.Name(), err)
			continue
		}
	}
	a.handles = map[uint32]*os.File{}
}

// AssertSafeOut refuses any output path inside the game directory.
func (a *Archive) AssertSafeOut(path string) error {
	abs, _ := filepath.Abs(path)
	game, _ := filepath.Abs(a.GameDir)
	if strings.HasPrefix(strings.ToLower(abs), strings.ToLower(game)) {
		return fmt.Errorf("refusing to write inside game dir: %s", path)
	}
	return nil
}

// Content reads and fully decodes one file's bytes (ICE-decrypt + LZ-decompress
// as needed). Stored files (CompSize==OrigSize) are returned verbatim.
func (a *Archive) Content(f PazFile) ([]byte, error) {
	fh, err := a.paz(f.PazNumber)
	if err != nil {
		return nil, err
	}
	data := make([]byte, f.CompSize)
	if _, err := fh.ReadAt(data, int64(f.Offset)); err != nil { // ReadAt is concurrency-safe
		return nil, err
	}

	if f.CompSize == f.OrigSize { // stored = plaintext
		return data, nil
	}

	needsDecrypt := true
	if len(data)%8 != 0 {
		needsDecrypt = false
	} else if len(data) >= 4 && string(data[:4]) == "PABR" {
		needsDecrypt = false
	}
	if needsDecrypt {
		data = a.ice.Decrypt(data)
	}

	isContainer := len(data) > 9 &&
		(data[0] == 0x6E || data[0] == 0x6F) &&
		bss.U32(data, 5) == f.OrigSize
	if isContainer {
		return Decompress(data, int(f.OrigSize)), nil
	}
	if int(f.OrigSize) <= len(data) {
		return data[:f.OrigSize], nil
	}
	return data, nil
}

// decodeInnerPABR removes the extra ICE layer used by stored PABR tables.
func decodeInnerPABR(data []byte) []byte {
	if len(data) < 4 || string(data[:4]) == "PABR" || len(data)%8 != 0 {
		return data
	}
	plain := NewICE(BDOICEKey).Decrypt(bytes.Clone(data))
	if len(plain) >= 4 && string(plain[:4]) == "PABR" {
		return plain
	}
	return data
}
