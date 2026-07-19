package output

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
)

// OwnershipManifestName is the durable list of files the last successful build owns.
const OwnershipManifestName = manifestName

// OwnershipManifest is the on-disk ownership record written by Publish.
type OwnershipManifest struct {
	Version int      `json:"version"`
	Files   []string `json:"files"`
}

// ReadOwnershipManifest loads dir/.build-outputs.json. It errors when the
// manifest is missing or invalid — compare tools need an explicit ownership set.
func ReadOwnershipManifest(dir string) (OwnershipManifest, error) {
	path := filepath.Join(dir, manifestName)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return OwnershipManifest{}, fmt.Errorf("no ownership manifest in %s (expected %s)", dir, manifestName)
	}
	if err != nil {
		return OwnershipManifest{}, fmt.Errorf("read ownership manifest: %w", err)
	}
	var m OwnershipManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return OwnershipManifest{}, fmt.Errorf("decode ownership manifest: %w", err)
	}
	if m.Version != 1 {
		return OwnershipManifest{}, fmt.Errorf("unsupported ownership manifest version %d", m.Version)
	}
	seen := make(map[string]bool, len(m.Files))
	for _, name := range m.Files {
		if _, err := cleanName(name); err != nil {
			return OwnershipManifest{}, fmt.Errorf("ownership manifest: %w", err)
		}
		if seen[name] {
			return OwnershipManifest{}, fmt.Errorf("ownership manifest contains duplicate path %q", name)
		}
		seen[name] = true
	}
	return m, nil
}

// ChangedFile is one owned path whose contents differ between two build dirs.
type ChangedFile struct {
	Name      string
	LeftSize  int64
	RightSize int64
	LeftSHA   string
	RightSHA  string
}

// DirDiff is the ownership-aware comparison of two successful build output dirs.
type DirDiff struct {
	OnlyLeft  []string
	OnlyRight []string
	Changed   []ChangedFile
	Same      int
}

// Equal reports whether the owned file sets and contents match.
func (d DirDiff) Equal() bool {
	return len(d.OnlyLeft) == 0 && len(d.OnlyRight) == 0 && len(d.Changed) == 0
}

// DiffOwned compares the files listed in each directory's ownership manifest.
// Unowned files (icons, worldmap tiles, provenance manifest.json, etc.) are ignored.
func DiffOwned(leftDir, rightDir string) (DirDiff, error) {
	leftMan, err := ReadOwnershipManifest(leftDir)
	if err != nil {
		return DirDiff{}, fmt.Errorf("left: %w", err)
	}
	rightMan, err := ReadOwnershipManifest(rightDir)
	if err != nil {
		return DirDiff{}, fmt.Errorf("right: %w", err)
	}

	leftSet := make(map[string]bool, len(leftMan.Files))
	for _, name := range leftMan.Files {
		leftSet[name] = true
	}
	rightSet := make(map[string]bool, len(rightMan.Files))
	for _, name := range rightMan.Files {
		rightSet[name] = true
	}

	var out DirDiff
	for _, name := range leftMan.Files {
		if !rightSet[name] {
			out.OnlyLeft = append(out.OnlyLeft, name)
		}
	}
	for _, name := range rightMan.Files {
		if !leftSet[name] {
			out.OnlyRight = append(out.OnlyRight, name)
		}
	}
	slices.Sort(out.OnlyLeft)
	slices.Sort(out.OnlyRight)

	both := make([]string, 0, len(leftMan.Files))
	for _, name := range leftMan.Files {
		if rightSet[name] {
			both = append(both, name)
		}
	}
	slices.Sort(both)

	for _, name := range both {
		leftPath := filepath.Join(leftDir, name)
		rightPath := filepath.Join(rightDir, name)
		ls, lhash, err := fileDigest(leftPath)
		if err != nil {
			return DirDiff{}, fmt.Errorf("left %s: %w", name, err)
		}
		rs, rhash, err := fileDigest(rightPath)
		if err != nil {
			return DirDiff{}, fmt.Errorf("right %s: %w", name, err)
		}
		if lhash == rhash {
			out.Same++
			continue
		}
		out.Changed = append(out.Changed, ChangedFile{
			Name:      name,
			LeftSize:  ls,
			RightSize: rs,
			LeftSHA:   lhash[:12],
			RightSHA:  rhash[:12],
		})
	}
	return out, nil
}

func fileDigest(path string) (size int64, shaHex string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return 0, "", err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return 0, "", err
	}
	return st.Size(), hex.EncodeToString(h.Sum(nil)), nil
}
