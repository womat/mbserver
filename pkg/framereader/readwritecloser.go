package framereader

import (
	"io"
	"time"
)

// ReadWriteCloser is a convenience type that implements io.ReadWriteCloser.
// Write calls flush reader before writing the prompt.
type ReadWriteCloser struct {
	reader *ReadCloser
	write  func([]byte) (int, error)
}

// NewReadWriteCloser creates a new response reader
//
// timeout is used to specify an
// overall timeout. If this timeout is encountered, io.EOF is returned.
//
// chunkTimeout is used to specify the max timeout between chunks of data once
// the response is started. If a delay of chunkTimeout is encountered, the response
// is considered finished and the Read returns.
func NewReadWriteCloser(iorw io.ReadWriteCloser, timeout time.Duration, interFrameDelay time.Duration) *ReadWriteCloser {
	return &ReadWriteCloser{
		write:  iorw.Write,
		reader: NewReadCloser(iorw, timeout, interFrameDelay),
	}
}

// Read response using interframedelay and timeout
func (rwc *ReadWriteCloser) Read(buffer []byte) (int, error) {
	return rwc.reader.Read(buffer)
}

// Write flushes all data from reader, and then passes through write call.
func (rwc *ReadWriteCloser) Write(buffer []byte) (int, error) {
	if _, err := rwc.reader.Flush(); err != nil {
		return 0, err
	}

	return rwc.write(buffer)
}

// Close stops the reader goroutines and closes the underlying resource.
// Safe to call multiple times.
func (rwc *ReadWriteCloser) Close() error {
	return rwc.reader.Close()
}
