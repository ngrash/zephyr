// Package stdstreams implements logging of stdout and stderr.
package stdstreams

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"io"
	"sync"
	"time"
)

type Stream uint8

func (s Stream) String() string {
	switch s {
	case Out:
		return "stdout"
	case Err:
		return "stderr"
	default:
		return "<unknown>"
	}
}

// Constants representing the types of streams.
const (
	Out Stream = 1
	Err        = 2
)

// A Line represents a line written to a standard stream.
type Line struct {
	Stream Stream
	Time   time.Time
	Text   string
}

// A Log collects output streams.
type Log struct {
	lines    []Line
	mutex    sync.Mutex
	callback func(newLine *Line)
	outW     *bufferedLineWriter
	errW     *bufferedLineWriter
}

// NewLog creates a new log.
func NewLog() *Log {
	return NewLogWithCallback(func(_ *Line) {})
}

func NewLogWithCallback(callback func(newLine *Line)) *Log {
	l := &Log{
		make([]Line, 0),
		sync.Mutex{},
		callback,
		nil,
		nil,
	}

	l.outW = l.makeWriter(Out)
	l.errW = l.makeWriter(Err)

	return l
}

// Stdout returns an io.Writer for collecting lines written to stdout.
func (l *Log) Stdout() io.Writer {
	return l.outW
}

// Stderr returns an io.Writer for collection lines written to stderr.
func (l *Log) Stderr() io.Writer {
	return l.errW
}

func (l *Log) makeWriter(stream Stream) *bufferedLineWriter {
	return newBufferedLineWriter(func(text string) {
		l.writeLine(stream, text)
	})
}

func (l *Log) writeLine(stream Stream, text string) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	line := Line{stream, time.Now(), text}
	l.lines = append(l.lines, line)
	l.callback(&line)
}

// Lines returns all lines written to the Log.
func (l *Log) Lines() []Line {
	return l.lines
}

// Flush forces Stdout and Stderr writer to write incomplete lines to the Log.
func (l *Log) Flush() {
	l.errW.Flush()
	l.outW.Flush()
}

// JSON returns a JSON representation of the lines contained in the Log.
// A number of lines might be skipped. Useful for polling new lines.
func (l *Log) JSON(skip int) ([]byte, error) {
	return json.Marshal(l.lines[skip:])
}

// MarshalBinary returns a binary representation of the Log.
func (l *Log) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(l.lines); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// UnmarshalBinary initializes the Log from the given binary representation.
func (l *Log) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	dec := gob.NewDecoder(r)
	return dec.Decode(&l.lines)
}

type bufferedLineWriter struct {
	buf *bytes.Buffer
	fn  func(text string)
}

func newBufferedLineWriter(fn func(text string)) *bufferedLineWriter {
	return &bufferedLineWriter{new(bytes.Buffer), fn}
}

func (w *bufferedLineWriter) Write(p []byte) (int, error) {
	n := 0
	for _, b := range p {
		n++
		if b == '\n' {
			w.fn(w.buf.String())
			w.buf.Reset()
		} else {
			w.buf.WriteByte(b)
		}
	}
	return n, nil
}

func (w *bufferedLineWriter) Flush() {
	if w.buf.Len() > 0 {
		w.fn(w.buf.String())
		w.buf.Reset()
	}
}
