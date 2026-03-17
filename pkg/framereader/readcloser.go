package framereader

import (
	"io"
	"time"
)

// ReadCloser is a convenience type that implements io.ReadWriter. Write
// calls flush reader before writing the prompt.
type ReadCloser struct {
	reader *Reader
	closer io.Closer
}

// NewReadCloser creates a new response reader
//
// timeout is used to specify an
// overall timeout. If this timeout is encountered, io.EOF is returned.
//
// chunkTimeout is used to specify the max timeout between chunks of data once
// the response is started. If a delay of chunkTimeout is encountered, the response
// is considered finished and the Read returns.
func NewReadCloser(iorw io.ReadCloser, timeout time.Duration, interframedelay time.Duration) *ReadCloser {
	return &ReadCloser{
		closer: iorw,
		reader: NewReader(iorw, timeout, interframedelay),
	}
}

// Read response using chunkTimeout and timeout
func (rc *ReadCloser) Read(buffer []byte) (int, error) {
	return rc.reader.Read(buffer)
}

// Close is a passthrough call.
func (rc *ReadCloser) Close() error {
	rc.reader.closed = true
	return rc.closer.Close()
}
