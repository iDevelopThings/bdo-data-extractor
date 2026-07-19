package tables

import (
	"os"
	"runtime"
	"testing"

	"github.com/idevelopthings/bdo-data-extractor/internal/paz"
)

// loadItemEnchantBytes reads itemenchant siblings from a live game install.
// Set BDO_GAME_DIR to override the default Steam path. Skips when unavailable.
func loadItemEnchantBytes(tb testing.TB) (offset, data []byte) {
	tb.Helper()
	gameDir := os.Getenv("BDO_GAME_DIR")
	if gameDir == "" {
		gameDir = paz.DefaultGameDir
	}
	if _, err := os.Stat(gameDir); err != nil {
		tb.Skipf("game dir unavailable (%s): %v", gameDir, err)
	}
	src, err := paz.OpenSource(gameDir)
	if err != nil {
		tb.Skipf("open game source: %v", err)
	}
	offset, err = src.Read("itemenchantoffset.dbss")
	if err != nil {
		tb.Fatalf("read itemenchantoffset: %v", err)
	}
	data, err = src.Read("itemenchant.dbss")
	if err != nil {
		tb.Fatalf("read itemenchant: %v", err)
	}
	return offset, data
}

func BenchmarkDecodeItemStats(b *testing.B) {
	offset, data := loadItemEnchantBytes(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := DecodeItemStats(offset, data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeItemStatsSerial(b *testing.B) {
	offset, data := loadItemEnchantBytes(b)
	prev := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(prev)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := DecodeItemStats(offset, data); err != nil {
			b.Fatal(err)
		}
	}
}
