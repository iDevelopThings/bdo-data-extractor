package paz

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/internal/bss"
)

// DefaultGameDir is the standard Steam install path.
const DefaultGameDir = `C:\Program Files (x86)\Steam\steamapps\common\Black Desert Online`

// PazFile is one 28-byte index record from pad00000.meta.
type PazFile struct {
	Hash      uint32
	FolderID  uint32
	FileID    uint32
	PazNumber uint32
	Offset    uint32
	CompSize  uint32
	OrigSize  uint32
}

// Index is the parsed pad00000.meta manifest.
type Index struct {
	Version     uint32
	PazCount    uint32
	Files       []PazFile
	FolderNames []string
	FileNames   []string

	byBase   map[string]int   // lazily built, lowercased basename -> first file index
	byFolder map[uint32][]int // lazily built, FolderID -> file indices (archive order)
}

// PazDir returns the Paz/ directory for a game install.
func PazDir(gameDir string) string { return filepath.Join(gameDir, "Paz") }

// PathOf reconstructs a file's full archive path.
func (ix *Index) PathOf(f PazFile) string {
	folder := fmt.Sprintf("<folder:%d>", f.FolderID)
	if int(f.FolderID) < len(ix.FolderNames) {
		folder = ix.FolderNames[f.FolderID]
	}
	name := fmt.Sprintf("<file:%d>", f.FileID)
	if int(f.FileID) < len(ix.FileNames) {
		name = ix.FileNames[f.FileID]
	}
	p := strings.TrimRight(folder, "/") + "/" + strings.TrimLeft(name, "/")
	return strings.ReplaceAll(p, "//", "/")
}

// buildIndex builds the basename and folder lookups once. It deliberately does NOT
// reconstruct every file's full path — that is ~800k string joins for data only a
// few thousand files ever need — so basenames come straight from FileNames and Path
// recomputes a single path on demand.
func (ix *Index) buildIndex() {
	if ix.byBase != nil {
		return
	}
	ix.byBase = make(map[string]int, len(ix.Files))
	ix.byFolder = make(map[uint32][]int)
	for i, f := range ix.Files {
		if int(f.FileID) < len(ix.FileNames) {
			name := ix.FileNames[f.FileID]
			base := strings.ToLower(name[strings.LastIndexByte(name, '/')+1:])
			if _, ok := ix.byBase[base]; !ok {
				ix.byBase[base] = i
			}
		}
		ix.byFolder[f.FolderID] = append(ix.byFolder[f.FolderID], i)
	}
}

// normFolder normalizes a folder path for suffix comparison: lowercase, forward
// slashes, no surrounding slashes.
func normFolder(s string) string {
	return strings.Trim(strings.ReplaceAll(strings.ToLower(s), `\`, "/"), "/")
}

// FilesUnder returns the indices of files directly inside the folder whose path
// ends with folderSuffix (e.g. "ui_html/xml/en"), in archive order. It enumerates
// that subtree from the folder index instead of scanning all Files, so callers that
// only care about one directory don't walk the whole 800k-file manifest. Multiple
// matching folders are merged back into global archive order.
func (ix *Index) FilesUnder(folderSuffix string) []int {
	ix.buildIndex()
	suffix := normFolder(folderSuffix)
	var out []int
	for id, name := range ix.FolderNames {
		if fn := normFolder(name); fn == suffix || strings.HasSuffix(fn, "/"+suffix) {
			out = append(out, ix.byFolder[uint32(id)]...)
		}
	}
	sort.Ints(out)
	return out
}

// Path returns file i's full archive path, recomputed on demand.
func (ix *Index) Path(i int) string {
	return ix.PathOf(ix.Files[i])
}

// Find returns the first file whose basename equals name (case-insensitive).
func (ix *Index) Find(name string) (PazFile, bool) {
	ix.buildIndex()
	if i, ok := ix.byBase[strings.ToLower(name)]; ok {
		return ix.Files[i], true
	}
	return PazFile{}, false
}

// LoadMeta parses pad00000.meta for the given game directory.
func LoadMeta(gameDir string) (*Index, error) {
	data, err := os.ReadFile(filepath.Join(PazDir(gameDir), "pad00000.meta"))
	if err != nil {
		return nil, err
	}
	ix := &Index{}
	ix.Version = bss.U32(data, 0)
	ix.PazCount = bss.U32(data, 4)
	cur := 8 + int(ix.PazCount)*12 // skip volume table (12B records)

	fileCount := bss.U32(data, cur)
	cur += 4
	ix.Files = make([]PazFile, fileCount)
	for i := range ix.Files {
		b := data[cur : cur+28]
		ix.Files[i] = PazFile{
			Hash:      bss.U32(b, 0),
			FolderID:  bss.U32(b, 4),
			FileID:    bss.U32(b, 8),
			PazNumber: bss.U32(b, 12),
			Offset:    bss.U32(b, 16),
			CompSize:  bss.U32(b, 20),
			OrigSize:  bss.U32(b, 24),
		}
		cur += 28
	}

	folderLen := int(bss.U32(data, cur))
	cur += 4
	folderRaw := append([]byte(nil), data[cur:cur+folderLen]...)
	cur += folderLen

	fileLen := int(bss.U32(data, cur))
	cur += 4
	fileRaw := append([]byte(nil), data[cur:cur+fileLen]...)

	ice := NewICE(BDOICEKey)
	ix.FolderNames = parseFolderTable(ice.Decrypt(folderRaw))
	ix.FileNames = parseNameTable(ice.Decrypt(fileRaw))

	return ix, nil
}

// folder table: repeating [8-byte header][NUL-terminated name]
func parseFolderTable(raw []byte) []string {
	var names []string
	cur := 0
	limit := len(raw) - 8
	for cur < limit {
		cur += 8
		nul := bytes.IndexByte(raw[cur:], 0)
		if nul == -1 {
			break
		}
		names = append(names, string(raw[cur:cur+nul]))
		cur += nul + 1
	}
	return names
}

// file table: repeating [NUL-terminated name]
func parseNameTable(raw []byte) []string {
	var names []string
	cur := 0
	for cur < len(raw) {
		nul := bytes.IndexByte(raw[cur:], 0)
		if nul == -1 {
			break
		}
		names = append(names, string(raw[cur:cur+nul]))
		cur += nul + 1
	}
	return names
}
