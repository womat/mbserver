package framereader

import (
	"io"
	"time"
)

// ReadWriter is a convenience type that implements io.ReadWriter. Write
// calls flush reader before writing the prompt.
type ReadWriter struct {
	write  func([]byte) (int, error)
	reader *Reader
}

// NewReadWriter creates a new response reader
func NewReadWriter(iorw io.ReadWriter, timeout time.Duration, interFrameDelay time.Duration) *ReadWriter {
	return &ReadWriter{
		write:  iorw.Write,
		reader: NewReader(iorw, timeout, interFrameDelay),
	}
}

// Read response
func (rw *ReadWriter) Read(buffer []byte) (int, error) {
	return rw.reader.Read(buffer)
}

// Write flushes all data from reader, and then passes through write call.
func (rw *ReadWriter) Write(buffer []byte) (int, error) {
	if _, err := rw.reader.Flush(); err != nil {
		return 0, err
	}

	return rw.write(buffer)
}
