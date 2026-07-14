package pipeline

import (
	"bufio"
	"encoding/binary"
	"os"
)

// Tile packs collapse a layer's tens of thousands of tile files into one file. Each
// filesystem object costs ~270µs to create even on fast storage, so 47k tiles spend
// seconds in write + delete syscalls that a single sequential file avoids entirely.
// The viewer reads tiles back by (z,x,y) via an in-memory index (see the matching
// reader in the viewer's asset handler).
//
// Layout (all integers little-endian):
//
//	magic   [8]byte "BDOTILE1"
//	blobs           concatenated WebP tiles, in arrival order
//	index           count uint32, then count × entry{ z uint8; x,y int32; off uint64; len uint32 }
//	footer  uint64   absolute file offset of the index (the final 8 bytes)
//
// A reader loads the last 8 bytes to find the index, reads the index into a map, then
// ReadAt's each blob on demand.
const (
	tilePackMagic     = "BDOTILE1"
	tilePackEntrySize = 21           // z(1) + x(4) + y(4) + off(8) + len(4)
	tilePackName      = "tiles.pack" // per-layer file name, sibling to meta.json
)

type packEntry struct {
	z      int
	x, y   int
	off    uint64
	length uint32
}

type packBlob struct {
	z      int
	coords [][2]int // one blob, indexed under one or many (x,y) — lets ocean fill dedup
	data   []byte
}

// tilePack collects tile blobs from many parallel encoders and writes them to one file
// sequentially. Encoders hand a blob to a buffered channel and return immediately; a
// single background goroutine owns the file, the running offset, and the index — so no
// encoder ever blocks on disk and there's no shared lock doing I/O (which would stall
// every worker whenever the buffer flushed).
type tilePack struct {
	ch   chan packBlob
	done chan error
}

func newTilePack(path string) (*tilePack, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	p := &tilePack{ch: make(chan packBlob, 1024), done: make(chan error, 1)}
	go p.run(f)
	return p, nil
}

// run drains the channel, appending each blob and recording its location, then writes
// the index + footer. It keeps draining after an error (so senders never deadlock) and
// reports the first error at close.
func (p *tilePack) run(f *os.File) {
	w := bufio.NewWriterSize(f, 8<<20)
	off := uint64(len(tilePackMagic))
	var idx []packEntry
	var err error

	if _, err = w.WriteString(tilePackMagic); err != nil {
		off = 0
	}
	for b := range p.ch {
		if err != nil {
			continue
		}
		if _, err = w.Write(b.data); err != nil {
			continue
		}
		for _, c := range b.coords {
			idx = append(idx, packEntry{z: b.z, x: c[0], y: c[1], off: off, length: uint32(len(b.data))})
		}
		off += uint64(len(b.data))
	}

	if err == nil {
		err = writePackIndex(w, idx, off)
	}
	if err == nil {
		err = w.Flush()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	p.done <- err
}

// add hands one tile's bytes to the writer. It never blocks on disk (only briefly if the
// channel buffer is full), and errors surface from close, not here.
func (p *tilePack) add(z, x, y int, data []byte) error {
	p.ch <- packBlob{z: z, coords: [][2]int{{x, y}}, data: data}
	return nil
}

// addMany stores data once and indexes it under every coord — for ocean fill, where
// thousands of empty cells share one tile, so the bytes aren't written thousands of times.
func (p *tilePack) addMany(z int, coords [][2]int, data []byte) {
	if len(coords) == 0 {
		return
	}
	p.ch <- packBlob{z: z, coords: coords, data: data}
}

// close signals no more tiles and waits for the writer to finish, returning its error.
func (p *tilePack) close() error {
	close(p.ch)
	return <-p.done
}

// writePackIndex appends the index (count + entries) and the footer offset.
func writePackIndex(w *bufio.Writer, idx []packEntry, indexOff uint64) error {
	var cnt [4]byte
	binary.LittleEndian.PutUint32(cnt[:], uint32(len(idx)))
	if _, err := w.Write(cnt[:]); err != nil {
		return err
	}
	var e [tilePackEntrySize]byte
	for _, en := range idx {
		e[0] = byte(en.z)
		binary.LittleEndian.PutUint32(e[1:5], uint32(int32(en.x)))
		binary.LittleEndian.PutUint32(e[5:9], uint32(int32(en.y)))
		binary.LittleEndian.PutUint64(e[9:17], en.off)
		binary.LittleEndian.PutUint32(e[17:21], en.length)
		if _, err := w.Write(e[:]); err != nil {
			return err
		}
	}
	var foot [8]byte
	binary.LittleEndian.PutUint64(foot[:], indexOff)
	_, err := w.Write(foot[:])
	return err
}
