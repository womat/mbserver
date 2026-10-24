package framereader

import (
	"io"
	"time"
)

// ReadCloser is a convenience type that implements io.ReadCloser.
type ReadCloser struct {
	reader *Reader
	close  func() error
}

// NewReadCloser creates a new response reader
//
// timeout is used to specify an
// overall timeout. If this timeout is encountered, ErrTimeout is returned.
//
// interFrameDelay is the silence that ends a frame: once data has started, a gap of
// interFrameDelay without new bytes completes the frame and Read returns it.
func NewReadCloser(iorw io.ReadCloser, timeout time.Duration, interFrameDelay time.Duration) *ReadCloser {
	return &ReadCloser{
		close:  iorw.Close,
		reader: NewReader(iorw, timeout, interFrameDelay),
	}
}

// Read returns the next frame, see Reader.Read.
func (rc *ReadCloser) Read(buffer []byte) (int, error) {
	return rc.reader.Read(buffer)
}

// Close stops the reader goroutines and closes the underlying resource.
// Safe to call multiple times.
func (rc *ReadCloser) Close() error {
	rc.reader.Close()
	return rc.close()
}

// Flush is used to flush any input data
func (rc *ReadCloser) Flush() (int, error) {
	return rc.reader.Flush()
}
