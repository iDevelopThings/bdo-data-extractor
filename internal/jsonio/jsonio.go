// Package jsonio reads and writes the CLI's JSON data files.
package jsonio

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"runtime"
	"sync"
)

// ReadFile decodes the JSON file at path into v.
func ReadFile(path string, v any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(bufio.NewReader(f)).Decode(v)
}

// WriteFile encodes v as JSON to path through a buffered writer. pretty indents
// the output; HTML escaping is always off (the data contains <, >, & literally).
func WriteFile(path string, v any, pretty bool) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriterSize(f, 1<<20)
	enc := json.NewEncoder(bw)
	enc.SetEscapeHTML(false)
	if pretty {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(v); err != nil {
		return err
	}
	return bw.Flush()
}

// parallelArrayMin is the element count above which WriteArray splits the encode
// across goroutines; below it the one-shot encoder is already cheap.
const parallelArrayMin = 4096

// WriteArray writes items as a JSON array to path, encoding chunks of the slice
// concurrently for large arrays. The output is byte-for-byte identical to
// WriteFile(path, items, pretty) — for the compact case it just parallelizes the
// reflection-heavy encode, the build's dominant cost when the array is hundreds of
// MB. The pretty case (and small/empty arrays) fall back to the streaming encoder.
func WriteArray[T any](path string, items []T, pretty bool) error {
	if pretty || len(items) < parallelArrayMin {
		return WriteFile(path, items, pretty)
	}

	workers := runtime.GOMAXPROCS(0)
	if workers > len(items) {
		workers = len(items)
	}
	chunks := make([][]byte, workers)
	errs := make([]error, workers)
	sz := (len(items) + workers - 1) / workers

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		lo := w * sz
		hi := lo + sz
		if hi > len(items) {
			hi = len(items)
		}
		if lo >= hi {
			continue
		}
		wg.Add(1)
		go func(w, lo, hi int) {
			defer wg.Done()
			chunks[w], errs[w] = encodeElems(items[lo:hi])
		}(w, lo, hi)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriterSize(f, 1<<20)
	bw.WriteByte('[')
	first := true
	for _, c := range chunks {
		if len(c) == 0 {
			continue
		}
		if !first {
			bw.WriteByte(',')
		}
		first = false
		bw.Write(c)
	}
	bw.WriteByte(']')
	bw.WriteByte('\n')
	return bw.Flush()
}

// encodeElems encodes each element as compact JSON (HTML escaping off) joined by
// commas, with no surrounding brackets — the interior of one array chunk. Matching
// json.Encoder's element bytes keeps WriteArray byte-identical to WriteFile.
func encodeElems[T any](items []T) ([]byte, error) {
	var out, one bytes.Buffer
	enc := json.NewEncoder(&one)
	enc.SetEscapeHTML(false)
	for i, it := range items {
		one.Reset()
		if err := enc.Encode(it); err != nil {
			return nil, err
		}
		b := one.Bytes()
		b = b[:len(b)-1] // drop the newline Encode appends
		if i > 0 {
			out.WriteByte(',')
		}
		out.Write(b)
	}
	return out.Bytes(), nil
}
