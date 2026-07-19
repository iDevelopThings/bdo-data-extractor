package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	gojson "github.com/goccy/go-json"
	"github.com/wI2L/jettison"

	"github.com/idevelopthings/bdo-data-extractor/internal/jsonio"
)

// Driver writes registered values to their staging paths. Write must create a
// regular file at path and may be called concurrently for normal artifacts. A
// transaction uses one driver, allowing callers to replace JSON with another
// serialization strategy.
type Driver interface {
	Write(path string, value any) error
}

// DriverFunc adapts a function to Driver.
type DriverFunc func(path string, value any) error

// Write calls f with the artifact's staging path and registered value.
func (f DriverFunc) Write(path string, value any) error {
	if f == nil {
		return fmt.Errorf("output driver function is nil")
	}
	return f(path, value)
}

// StandardJSONDriver writes registered values with encoding/json.
type StandardJSONDriver struct {
	Pretty bool
}

// NewStandardJSONDriver returns an encoding/json output driver.
func NewStandardJSONDriver(pretty bool) StandardJSONDriver {
	return StandardJSONDriver{Pretty: pretty}
}

// Write encodes value as JSON, using the parallel array path for JSONArray values.
func (d StandardJSONDriver) Write(path string, value any) error {
	if array, ok := value.(jsonArrayValue); ok {
		return array.writeStandardJSON(path, d.Pretty)
	}
	return jsonio.WriteFile(path, value, d.Pretty)
}

// GoccyJSONDriver writes registered values with goccy/go-json.
type GoccyJSONDriver struct {
	Pretty bool
}

// NewGoccyJSONDriver returns a goccy/go-json output driver.
func NewGoccyJSONDriver(pretty bool) GoccyJSONDriver {
	return GoccyJSONDriver{Pretty: pretty}
}

// Write encodes value as JSON, using the parallel array path for JSONArray values.
func (d GoccyJSONDriver) Write(path string, value any) error {
	if array, ok := value.(jsonArrayValue); ok {
		return array.writeGoccyJSON(path, d.Pretty)
	}
	return jsonio.WriteFileWith(path, value, d.Pretty, newGoccyEncoder)
}

func newGoccyEncoder(w io.Writer) jsonio.Encoder {
	return gojson.NewEncoder(w)
}

// JettisonJSONDriver writes registered values with wI2L/jettison.
type JettisonJSONDriver struct {
	Pretty bool
}

// NewJettisonJSONDriver returns a wI2L/jettison output driver.
func NewJettisonJSONDriver(pretty bool) JettisonJSONDriver {
	return JettisonJSONDriver{Pretty: pretty}
}

// Write encodes value as JSON, using the parallel array path for JSONArray values.
func (d JettisonJSONDriver) Write(path string, value any) error {
	if array, ok := value.(jsonArrayValue); ok {
		return array.writeJettisonJSON(path, d.Pretty)
	}
	return jsonio.WriteFileWith(path, value, d.Pretty, newJettisonEncoder)
}

func newJettisonEncoder(w io.Writer) jsonio.Encoder {
	return &jettisonEncoder{w: w, escapeHTML: true}
}

// jettisonEncoder adapts Jettison's marshal API to the streaming subset used by
// jsonio. Each Encode call remains independent, as required by the array chunker.
type jettisonEncoder struct {
	w          io.Writer
	escapeHTML bool
	prefix     string
	indent     string
	buf        []byte
}

func (e *jettisonEncoder) Encode(value any) error {
	var (
		data []byte
		err  error
	)
	if e.escapeHTML {
		data, err = jettison.Append(e.buf[:0], value)
	} else {
		data, err = jettison.AppendOpts(e.buf[:0], value, jettison.NoHTMLEscaping())
	}
	if err != nil {
		return err
	}
	if e.indent != "" {
		var indented bytes.Buffer
		if err := json.Indent(&indented, data, e.prefix, e.indent); err != nil {
			return err
		}
		data = indented.Bytes()
	}
	data = append(data, '\n')
	if e.indent == "" {
		e.buf = data
	}
	n, err := e.w.Write(data)
	if err == nil && n != len(data) {
		return io.ErrShortWrite
	}
	return err
}

func (e *jettisonEncoder) SetEscapeHTML(enabled bool) {
	e.escapeHTML = enabled
}

func (e *jettisonEncoder) SetIndent(prefix, indent string) {
	e.prefix = prefix
	e.indent = indent
}

type jsonArrayValue interface {
	writeStandardJSON(path string, pretty bool) error
	writeGoccyJSON(path string, pretty bool) error
	writeJettisonJSON(path string, pretty bool) error
}

// JSONArray wraps a slice for the JSON drivers' parallel array encoder.
type JSONArray[T any] struct {
	values []T
}

// NewJSONArray wraps values for a JSON driver. Register the result as exclusive
// because its encoder uses all available CPUs itself.
func NewJSONArray[T any](values []T) JSONArray[T] {
	return JSONArray[T]{values: values}
}

func (a JSONArray[T]) writeStandardJSON(path string, pretty bool) error {
	return jsonio.WriteArray(path, a.values, pretty)
}

func (a JSONArray[T]) writeGoccyJSON(path string, pretty bool) error {
	return jsonio.WriteArrayWith(path, a.values, pretty, newGoccyEncoder)
}

func (a JSONArray[T]) writeJettisonJSON(path string, pretty bool) error {
	return jsonio.WriteArrayWith(path, a.values, pretty, newJettisonEncoder)
}
