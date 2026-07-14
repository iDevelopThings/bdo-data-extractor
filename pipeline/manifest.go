package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ManifestName is the provenance file RunAll writes into the data dir, recording
// what the extraction was produced from so a consumer can tell when the cached
// output is stale (the game patched, or the embedding app updated) and rerun.
const ManifestName = "manifest.json"

// Manifest describes one extraction run's provenance.
type Manifest struct {
	GameFingerprint string `json:"gameFingerprint"` // identifies the game data — see GameFingerprint
	AppVersion      string `json:"appVersion"`      // the embedding app's version at extraction time
	Lang            string `json:"lang"`
	Region          string `json:"region"`      // game service region
	ExtractedAt     string `json:"extractedAt"` // RFC 3339
}

// GameFingerprint returns a short hash identifying the game's current extractable
// data. It hashes Paz/pad00000.meta — the archive index, which changes whenever
// archived content is added, removed, or repacked — together with ads_version, so
// it moves whenever a patch would change what the extractor produces.
func GameFingerprint(gameDir string) (string, error) {
	h := sha256.New()

	f, err := os.Open(filepath.Join(gameDir, "Paz", "pad00000.meta"))
	if err != nil {
		return "", fmt.Errorf("game fingerprint: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("game fingerprint: %w", err)
	}

	// ads_version is best-effort context — its absence shouldn't fail the fingerprint.
	if v, err := os.ReadFile(filepath.Join(gameDir, "ads_version")); err == nil {
		h.Write(v)
	}

	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// IconCodecVersion identifies the icon-producing logic (which items get icons, the
// icon paths, the DDS decode, the output encoding). BUMP IT whenever a change alters
// icon output — otherwise stale icons are kept.
const IconCodecVersion = 2

// WorldMapCodecVersion identifies the world-map-producing logic (the tile pyramid,
// its encoding, and the tiles.pack layout). BUMP IT whenever a change alters the
// world-map output — otherwise a stale pyramid is kept.
const WorldMapCodecVersion = 1

// asset is one provenance-tracked output pass: the derived art RunAll can reuse
// across an app update that didn't touch the game or the pass's own code. Each
// records the key it was produced for in a file beside its output, written only
// after the pass succeeds — so a crashed run never claims the output is complete.
type asset struct {
	// name labels the pass in logs and salts its key.
	name string
	// version is the pass's codec version; bumping it invalidates existing output.
	version int
	// dir is the output directory under dataDir; its absence means "not produced".
	dir string
	// keyFile is where the produced-for key is recorded, under dataDir.
	keyFile string
}

var (
	iconAsset     = asset{name: "icons", version: IconCodecVersion, dir: "icons", keyFile: ".icon_provenance"}
	worldMapAsset = asset{name: "world map", version: WorldMapCodecVersion, dir: "worldmap", keyFile: ".worldmap_provenance"}
)

// key identifies the output valid for a game install: the game fingerprint (the
// archive art) plus the pass's codec version. It deliberately excludes the app
// version — derived art doesn't change just because the embedding app updated.
func (a asset) key(gameDir string) (string, error) {
	fp, err := GameFingerprint(gameDir)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(fp + "|" + a.name + "|" + strconv.Itoa(a.version)))
	return hex.EncodeToString(h[:])[:16], nil
}

// fresh reports whether dataDir already holds this pass's output for the current
// game + codec version, so RunAll can skip it. It is false if the provenance is
// missing or mismatched, or if the output directory itself is gone.
func (a asset) fresh(dataDir, gameDir string) bool {
	key, err := a.key(gameDir)
	if err != nil {
		return false
	}
	got, err := os.ReadFile(filepath.Join(dataDir, a.keyFile))
	if err != nil || strings.TrimSpace(string(got)) != key {
		return false
	}
	if fi, err := os.Stat(filepath.Join(dataDir, a.dir)); err != nil || !fi.IsDir() {
		return false
	}
	return true
}

// stamp records the current key, marking the output complete for this game + codec
// version. Call it only after the producing pass succeeds.
func (a asset) stamp(dataDir, gameDir string) error {
	key, err := a.key(gameDir)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, a.keyFile), []byte(key), 0o644)
}

// writeManifest records an extraction's provenance in dataDir/manifest.json.
func writeManifest(dataDir, gameDir, appVersion, lang, region string) error {
	fp, err := GameFingerprint(gameDir)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(Manifest{
		GameFingerprint: fp,
		AppVersion:      appVersion,
		Lang:            lang,
		Region:          region,
		ExtractedAt:     time.Now().UTC().Format(time.RFC3339),
	}, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dataDir, ManifestName), b, 0o644)
}

// ReadManifest loads dataDir/manifest.json. A non-nil error (including a missing
// file) means the extraction's provenance is unknown — treat the data as stale.
func ReadManifest(dataDir string) (Manifest, error) {
	b, err := os.ReadFile(filepath.Join(dataDir, ManifestName))
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return Manifest{}, err
	}

	return m, nil
}

// NeedsExtraction reports whether the extracted data in dataDir is stale for the
// given game install and app version, with a human-readable reason. It's a boot
// check for an existing dataset (a missing dataset is a separate first-run case).
//
// It returns true when the manifest is missing, the app has updated since
// extraction, or the game data has changed. If the game dir can't be read it
// returns false — a re-extraction isn't possible anyway, so the caller should keep
// using the data it already has rather than being forced back into setup offline.
func NeedsExtraction(dataDir, gameDir, appVersion string) (stale bool, reason string) {
	fp, err := GameFingerprint(gameDir)
	if err != nil {
		return false, ""
	}
	m, err := ReadManifest(dataDir)
	if err != nil {
		return true, "no extraction manifest"
	}
	if m.AppVersion != appVersion {
		return true, fmt.Sprintf("app updated (%s -> %s)", m.AppVersion, appVersion)
	}
	if m.GameFingerprint != fp {
		return true, "game data changed"
	}

	return false, ""
}
