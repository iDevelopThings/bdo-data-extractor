// Package jsonio reads and writes the CLI's JSON data files.
package jsonio

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"os"
	"runtime"

	"github.com/dgravesa/go-parallel/parallel"
)

// Encoder is the streaming subset shared by encoding/json-compatible codecs.
type Encoder interface {
	Encode(any) error
	SetEscapeHTML(bool)
	SetIndent(prefix, indent string)
}

// EncoderFactory constructs a streaming encoder for w.
type EncoderFactory func(w io.Writer) Encoder

// StandardEncoder constructs the standard library JSON encoder.
func StandardEncoder(w io.Writer) Encoder {
	return json.NewEncoder(w)
}

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
	return WriteFileWith(path, v, pretty, StandardEncoder)
}

// WriteFileWith encodes v using newEncoder through the same buffered file path
// and formatting policy as WriteFile.
func WriteFileWith(path string, v any, pretty bool, newEncoder EncoderFactory) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriterSize(f, 1<<20)
	enc := newEncoder(bw)
	enc.SetEscapeHTML(false)
	if pretty {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(v); err != nil {
		return err
	}
	return bw.Flush()
}

// Marshal encodes v the same way WriteFile does (HTML escaping off, optional
// indent) but returns the bytes, for callers that hand the JSON to something other
// than a file. The output matches WriteFile(path, v, pretty) byte-for-byte.
func Marshal(v any, pretty bool) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if pretty {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
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
	return WriteArrayWith(path, items, pretty, StandardEncoder)
}

// WriteArrayWith is WriteArray with a caller-supplied compatible encoder.
func WriteArrayWith[T any](path string, items []T, pretty bool, newEncoder EncoderFactory) error {
	if pretty || len(items) < parallelArrayMin {
		return WriteFileWith(path, items, pretty, newEncoder)
	}

	workers := runtime.GOMAXPROCS(0)
	if workers > len(items) {
		workers = len(items)
	}
	chunks := make([][]byte, workers)
	errs := make([]error, workers)
	sz := (len(items) + workers - 1) / workers

	parallel.WithNumGoroutines(workers).For(workers, func(w, _ int) {
		lo := w * sz
		hi := min(lo+sz, len(items))
		if lo < hi {
			chunks[w], errs[w] = encodeElems(items[lo:hi], newEncoder)
		}
	})
	for _, err := range errs {
		if err != nil {
			return err
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			log.Fatalf("[JsonIO.WriteArray] failed to close %s: %v", path, err)
		}
	}(f)
	bw := bufio.NewWriterSize(f, 1<<20)
	err = bw.WriteByte('[')
	if err != nil {
		return err
	}
	first := true
	for _, c := range chunks {
		if len(c) == 0 {
			continue
		}
		if !first {
			err = bw.WriteByte(',')
			if err != nil {
				return err
			}
		}
		first = false
		_, err = bw.Write(c)
		if err != nil {
			return err
		}
	}
	err = bw.WriteByte(']')
	if err != nil {
		return err
	}
	err = bw.WriteByte('\n')
	if err != nil {
		return err
	}
	return bw.Flush()
}

// encodeElems encodes each element as compact JSON (HTML escaping off) joined by
// commas, with no surrounding brackets — the interior of one array chunk. Matching
// json.Encoder's element bytes keeps WriteArray byte-identical to WriteFile.
func encodeElems[T any](items []T, newEncoder EncoderFactory) ([]byte, error) {
	var out, one bytes.Buffer
	enc := newEncoder(&one)
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
