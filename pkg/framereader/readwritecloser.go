package framereader

import (
	"io"
	"time"
)

// ReadWriteCloser is a convenience type that implements io.ReadWriteCloser.
// Write calls flush reader before writing the prompt.
type ReadWriteCloser struct {
	reader *Reader
	writer io.Writer
	closer io.Closer
}

// NewReadWriteCloser creates a new response reader
//
// timeout is used to specify an
// overall timeout. If this timeout is encountered, io.EOF is returned.
//
// chunkTimeout is used to specify the max timeout between chunks of data once
// the response is started. If a delay of chunkTimeout is encountered, the response
// is considered finished and the Read returns.
func NewReadWriteCloser(iorw io.ReadWriteCloser, timeout time.Duration, interframedelay time.Duration) *ReadWriteCloser {
	return &ReadWriteCloser{
		closer: iorw,
		writer: iorw,
		reader: NewReader(iorw, timeout, interframedelay),
	}
}

// Read response using interframedelay and timeout
func (rwc *ReadWriteCloser) Read(buffer []byte) (int, error) {
	return rwc.reader.Read(buffer)
}

// Write flushes all data from reader, and then passes through write call.
func (rwc *ReadWriteCloser) Write(buffer []byte) (int, error) {
	n, err := rwc.reader.Flush()
	if err != nil {
		return n, err
	}

	return rwc.writer.Write(buffer)
}

// Close is a passthrough call.
func (rwc *ReadWriteCloser) Close() error {
	rwc.reader.closed = true
	return rwc.closer.Close()
}
