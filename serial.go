package mbserver

import (
	"io"
	"time"
)

// SerialPort is the interface for a serial port.
// Implemented by go.bug.st/serial.Port.
type SerialPort interface {
	io.ReadWriteCloser
	SetReadTimeout(t time.Duration) error
}
