package framereader

import (
	"errors"
	"io"
	"time"
)

// framesize is the max buffer size
const framesize = 255

// Reader is used for prompt/response communication protocols where a prompt
// is sent, and some time later a response is received. Typically, the target takes
// some amount to formulate the response, and then streams it out. There are two delays:
// an overall timeout, and the time interval between two data packets (inter frame delay).
// This delay is measured from the last received byte to the first next received byte.
// The thought is that once you received the 1st byte, all the data should stream out
// continuously and a short timeout can be used to determine the end of the packet.
type Reader struct {
	reader          io.Reader
	timeout         time.Duration
	interframedelay time.Duration
	dataChan        chan []byte
	closed          bool
}

// NewReader creates a new response reader.
//
// timeout is used to specify an
// overall timeout. If this timeout is encountered, io.EOF is returned.
//
// chunkTimeout is used to specify the max timeout between chunks of data once
// the response is started. If a delay of chunkTimeout is encountered, the response
// is considered finished and the Read returns.
func NewReader(reader io.Reader, timeout time.Duration, interframedelay time.Duration) *Reader {
	r := Reader{
		reader:          reader,
		timeout:         timeout,
		interframedelay: interframedelay,
		dataChan:        make(chan []byte, 5),
	}
	// we have to start a reader goroutine here that lives for the life
	// of the reader because there is no
	// way to stop a blocked goroutine
	go r.framereader()

	return &r
}

// Read response
func (r *Reader) Read(buffer []byte) (n int, err error) {
	if len(buffer) <= 0 {
		return 0, errors.New("must supply non-zero length buffer")
	}

	timeout := time.NewTimer(r.timeout)

	select {
	case b, ok := <-r.dataChan:
		n = copy(buffer, b)
		if !ok {
			err = io.EOF
		}
	case <-timeout.C:
		err = io.EOF
	}

	return
}

// Flush is used to flush any input data
func (r *Reader) Flush() (n int, err error) {
	frames := 0
	timeout := time.NewTimer(r.interframedelay)

	defer func() {
		debuglog.Printf("drop %v frames (%v bytes)\n", frames, n)
	}()

	for {
		select {
		case newData, ok := <-r.dataChan:
			n += len(newData)
			tracelog.Printf("drop frame with %v bytes\n", len(newData))

			if !ok {
				return n, io.EOF
			}

			frames++
			timeout.Reset(r.interframedelay)

		case <-timeout.C:
			return n, nil
		}
	}
}
