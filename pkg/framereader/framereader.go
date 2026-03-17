package framereader

import (
	"encoding/hex"
	"io"
	"time"
)

func (r *Reader) framereader() {
	defer func() {
		infolog.Println("stop frame reader services")
		close(r.dataChan)
	}()
	data := make(chan []byte)

	go func() { // this goroutine reads data from *Reader, until reader is closed reader.Closed
		defer func() {
			infolog.Println("stop serialport reader")
			close(data)
		}()
		for !r.closed {
			buffer := make([]byte, framesize)
			if n, _ := r.reader.Read(buffer); n > 0 {
				tracelog.Printf("read %v byte(s) from serial port: %v\n", n, hex.EncodeToString(buffer[:n]))
				data <- buffer[:n]
			}
		}
	}()

	for !r.closed { // this goroutine reads data from *Reader, until reader is closed reader.Closed
		var icd, icdmax time.Duration
		buffer := make([]byte, framesize)

		n, err := func(frame []byte) (int, error) {
			count := 0
			for { // this goroutine reads a frame: appends data from serial port, until the delay between characters greater then the interframedelay
				t := time.Now()
				timeout := time.NewTimer(r.interframedelay)
				select {
				case chunk, ok := <-data:
					if count > 0 {
						icd = time.Since(t)
					}

					tracelog.Printf("read new chunk (icd): (%v) %v\n", icd, hex.EncodeToString(chunk[:]))

					if icd > icdmax {
						// calc icdmax of the received Frame
						icdmax = icd
					}

					for i := 0; count < len(frame) && i < len(chunk); i++ {
						frame[count] = chunk[i]
						count++
					}

					if !ok { // the channel is closed, no more characters can received
						tracelog.Println("the channel is closed, no more characters can received, exit with EOF")
						return count, io.EOF
					}

				case <-timeout.C:
					icd = time.Since(t)
					return count, nil
				}
			}
		}(buffer)

		if err != nil {
			infolog.Println("the channel is closed, no more characters can received, stop service")
			return
		}

		if n > 0 {
			// New Frame received
			debuglog.Printf("read new frame (ifd/icdmax): (%v/%v) %v\n", icd, icdmax, hex.EncodeToString(buffer[:n]))
			r.dataChan <- buffer[:n]
		}
	}
}
