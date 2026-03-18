package framereader

import (
	"errors"
	"io"
	"time"
)

func (r *Reader) frameReader() {
	defer func() {
		close(r.data)
	}()

	ioData := make(chan []byte)

	go func() { // this goroutine reads data from *Reader, until reader is closed reader.Closed
		defer func() {
			close(ioData)
		}()
		ioBuffer := make([]byte, frameSize)
		for {
			select {
			case <-r.stop:
				return
			default:
			}

			n, err := r.ioRead(ioBuffer)
			if err != nil && !errors.Is(err, io.EOF) {
				// ioReader should not be closed, but if it is, we should stop the framereader goroutine
				return
			}

			// n=0 without error is valid per io.Reader spec but should never occur on a serial port.
			// Skip to avoid sending empty slices downstream.
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, ioBuffer[:n])
				select {
				case ioData <- chunk:
				case <-r.stop:
					return
				}
			}
		}
	}()

	buffer := make([]byte, 0, frameSize)

	// reads data from data channel until channel is closed
	for {
		timeout := time.NewTimer(r.interFrameDelay)
		select {
		case chunk, ok := <-ioData:
			if !timeout.Stop() {
				<-timeout.C
			}

			if !ok {
				// the channel is closed, no more characters can received
				return
			}

			buffer = append(buffer, chunk...)

		case <-r.stop:
			if !timeout.Stop() {
				<-timeout.C
			}
			return

		case <-timeout.C:
			if len(buffer) > 0 {
				// inter-frame delay expired → frame complete, forward to dataChan
				r.data <- buffer
				// create new buffer for next frame
				buffer = make([]byte, 0, frameSize)
			}
		}
	}
}
