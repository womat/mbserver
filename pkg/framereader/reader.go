package framereader

import (
	"errors"
	"io"
	"time"
)

// frameSize is the max buffer size
const frameSize = 255

// Reader is used for prompt/response communication protocols where a prompt
// is sent, and some time later a response is received. Typically, the target takes
// some amount to formulate the response, and then streams it out. There are two delays:
// an overall timeout, and the time interval between two data packets (inter frame delay).
// This delay is measured from the last received byte to the first next received byte.
// The thought is that once you received the 1st byte, all the data should stream out
// continuously and a short timeout can be used to determine the end of the packet.
type Reader struct {
	ioRead          func(p []byte) (int, error)
	timeout         time.Duration
	interFrameDelay time.Duration
	data            chan []byte
	stop            chan struct{}
}

var ErrZeroBuffer = errors.New("must supply non-zero length buffer")

// NewReader creates a new response reader.
//
// timeout is used to specify an
// overall timeout. If this timeout is encountered, io.EOF is returned.
//
// chunkTimeout is used to specify the max timeout between chunks of data once
// the response is started. If a delay of chunkTimeout is encountered, the response
// is considered finished and the Read returns.
func NewReader(ioReader io.Reader, timeout time.Duration, interFrameDelay time.Duration) *Reader {
	r := Reader{
		ioRead:          ioReader.Read,
		timeout:         timeout,
		interFrameDelay: interFrameDelay,
		data:            make(chan []byte),
		stop:            make(chan struct{}),
	}
	// we have to start a reader goroutine here that lives for the life
	// of the reader because there is no
	// way to stop a blocked goroutine
	go r.frameReader()
	return &r
}

// Read response
func (r *Reader) Read(buffer []byte) (int, error) {
	if len(buffer) <= 0 {
		return 0, ErrZeroBuffer
	}

	timeout := time.NewTimer(r.timeout)
	defer timeout.Stop()

	select {
	case b, ok := <-r.data:
		if !ok {
			return 0, io.EOF
		}

		n := copy(buffer, b)
		return n, nil

	case <-r.stop:
		return 0, io.EOF

	case <-timeout.C:
		return 0, io.EOF
	}
}

// Flush is used to flush any input data
func (r *Reader) Flush() (int, error) {
	timeout := time.NewTimer(r.interFrameDelay)
	defer timeout.Stop()

	n := 0
	for {
		select {
		case newData, ok := <-r.data:
			if !ok {
				return 0, io.EOF
			}

			n += len(newData)
			if !timeout.Stop() {
				<-timeout.C
			}
			timeout.Reset(r.interFrameDelay)

		case <-r.stop:
			return n, nil
		case <-timeout.C:
			return n, nil
		}
	}
}
