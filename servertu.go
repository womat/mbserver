package mbserver

import (
	"context"
	"io"
	"time"

	"github.com/womat/mbserver/pkg/framereader"
	"go.bug.st/serial"
)

// Parity describes a serial port parity setting
type Parity int

// StopBits describe a serial port stop bits setting
type StopBits int

const (
	NoParity    = Parity(serial.NoParity)
	OddParity   = Parity(serial.OddParity)
	EvenParity  = Parity(serial.EvenParity)
	MarkParity  = Parity(serial.MarkParity)
	SpaceParity = Parity(serial.SpaceParity)

	OneStopBit           = StopBits(serial.OneStopBit)
	OnePointFiveStopBits = StopBits(serial.OnePointFiveStopBits)
	TwoStopBits          = StopBits(serial.TwoStopBits)
)

type SerialConfig struct {
	BaudRate int
	DataBits int
	StopBits StopBits
	Parity   Parity
	Timeout  time.Duration
}

// t3.5 bei 9600 Baud ≈ 4ms
func interframeDelay(baudRate int) time.Duration {
	if baudRate <= 19200 {
		// Spec: 3.5 * (1/baudRate) * 11 bits
		return time.Duration(38500000/baudRate) * time.Nanosecond
	}
	// Bei höheren Baudraten fix 1.75ms laut Modbus-Spec
	return 1750 * time.Microsecond
}

// ListenRTU starts the Modbus server listening to a serial device.
// Example:
//
//	 usage of package "go.bug.st/serial"
//		port, err := serial.Open("/dev/ttyUSB0", &serial.Mode{
//			BaudRate: 9600,
//			DataBits: 8,
//			StopBits: serial.OneStopBit,
//			Parity:   serial.NoParity,
//			})
//		s.ListenRTU(port, 9600)
func (s *Server) ListenRTU(ctx context.Context, port string, config SerialConfig) error {

	p, err := serial.Open(port, &serial.Mode{
		BaudRate: config.BaudRate,
		DataBits: config.DataBits,
		StopBits: serial.StopBits(config.StopBits),
		Parity:   serial.Parity(config.Parity),
	})

	if err != nil {
		return err
	}
	if err = p.SetReadTimeout(serial.NoTimeout); err != nil {
		return err
	}

	t := config.Timeout
	if t <= 0 {
		t = time.Second * 5
	}

	rwc := framereader.NewReadWriteCloser(p, t, interframeDelay(config.BaudRate))
	s.mu.Lock()
	s.ports = append(s.ports, rwc)
	s.mu.Unlock()

	s.wg.Add(1)
	go s.acceptSerialRequests(ctx, rwc)
	return nil
}

func (s *Server) acceptSerialRequests(ctx context.Context, reader io.ReadWriteCloser) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		frame := make([]byte, 512)
		n, err := reader.Read(frame)
		if err != nil {
			s.log.Error("Error reading serial port", "error", err)
			return
		}

		if n == 0 {
			// n=0 without error is valid per io.Reader spec but should never occur on a serial port.
			// Sleep prevents a CPU spin in case of a misbehaving driver or port.
			s.log.Warn("Serial port read returned 0 bytes without error, retrying")
			time.Sleep(time.Millisecond)
			continue
		}

		rtuFrame, err := NewRTUFrame(frame[:n])
		if err != nil {
			s.log.Error("Error parsing RTU frame", "error", err)
			continue
		}
		s.request <- Request{reader, rtuFrame}
	}
}
