package mbserver

import (
	"errors"
	"io"

	"github.com/womat/debug"
)

// ListenRTU starts the Modbus server listening to a serial device.
// For example:  err := s.ListenRTU(&serial.Config{Address: "/dev/ttyUSB0"})
func (s *Server) ListenRTU(port io.ReadWriteCloser) (err error) {
	s.ports = append(s.ports, port)
	go s.acceptSerialRequests(port)
	return err
}

func (s *Server) acceptSerialRequests(port io.ReadWriteCloser) {
	for {
		buffer := make([]byte, 512)

		bytesRead, err := port.Read(buffer)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				debug.ErrorLog.Printf("serial read error %v", err)
			}
			continue
		}

		if bytesRead != 0 {
			// Set the length of the packet to the number of read bytes.
			packet := buffer[:bytesRead]

			frame, err := NewRTUFrame(packet)
			if err != nil {
				debug.WarningLog.Printf("bad serial frame error %v", err)
				continue
			}

			request := &Request{port, frame}

			s.requestChan <- request
		}
	}
}
