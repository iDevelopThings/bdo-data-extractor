package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// writeManifest records an extraction's provenance in dataDir/manifest.json.
func writeManifest(dataDir, gameDir, appVersion, lang string) error {
	fp, err := GameFingerprint(gameDir)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(Manifest{
		GameFingerprint: fp,
		AppVersion:      appVersion,
		Lang:            lang,
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
