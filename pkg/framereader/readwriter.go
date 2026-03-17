package framereader

import (
	"io"
	"time"
)

// ReadWriter is a convenience type that implements io.ReadWriter. Write
// calls flush reader before writing the prompt.
type ReadWriter struct {
	writer io.Writer
	reader *Reader
}

// NewReadWriter creates a new response reader
func NewReadWriter(iorw io.ReadWriter, timeout time.Duration, interframedelay time.Duration) *ReadWriter {
	return &ReadWriter{
		writer: iorw,
		reader: NewReader(iorw, timeout, interframedelay),
	}
}

// Read response
func (rw *ReadWriter) Read(buffer []byte) (int, error) {
	return rw.reader.Read(buffer)
}

// Write flushes all data from reader, and then passes through write call.
func (rw *ReadWriter) Write(buffer []byte) (int, error) {
	n, err := rw.reader.Flush()
	if err != nil {
		return n, err
	}

	return rw.writer.Write(buffer)
}
