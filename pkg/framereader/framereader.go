package framereader

import (
	"errors"
	"io"
	"time"
)

// stopTimer stops t and drains its channel if it has already fired.
// Safe to call regardless of whether the channel has already been consumed.
func stopTimer(t *time.Timer) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
}

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
	timeout := time.NewTimer(r.interFrameDelay)
	defer stopTimer(timeout)

	for {
		select {
		case chunk, ok := <-ioData:
			if !ok {
				// ioData closed — no more data will arrive; defer stopTimer handles cleanup.
				return
			}
			// Drain timer before resetting so Reset is safe to call.
			stopTimer(timeout)
			buffer = append(buffer, chunk...)
			timeout.Reset(r.interFrameDelay)

		case <-r.stop:
			return

		case <-timeout.C:
			if len(buffer) > 0 {
				// Inter-frame delay expired → frame complete, forward to data chan.
				// Use select so a concurrent Close() is not blocked by a slow consumer.
				select {
				case r.data <- buffer:
				case <-r.stop:
					return
				}
				buffer = make([]byte, 0, frameSize)
			}
			timeout.Reset(r.interFrameDelay)
		}
	}
}
