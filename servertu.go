package mbserver

import (
	"io"
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
			if err != io.EOF {
				errorlog.Printf("serial read error %v\n", err)
			}
			continue
		}

		if bytesRead != 0 {

			// Set the length of the packet to the number of read bytes.
			packet := buffer[:bytesRead]

			frame, err := NewRTUFrame(packet)
			if err != nil {
				warninglog.Printf("bad serial frame error %v\n", err)
				continue
			}

			request := &Request{port, frame}

			s.requestChan <- request
		}
	}
}
