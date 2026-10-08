package mbserver

import (
	"context"
	"errors"
	"io"
	"sync"
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

// SerialConfig holds the settings of a serial port for ListenRTU.
type SerialConfig struct {
	BaudRate int
	DataBits int
	StopBits StopBits
	Parity   Parity
	Timeout  time.Duration // read timeout, 5 s if unset; the server keeps listening after it

	// InterFrameDelay is the silence that ends a request. 0 means t3.5 of the Modbus
	// specification (about 4 ms at 9600 baud). USB adapters deliver bytes in packets with
	// gaps of several milliseconds; raise it (e.g. to 20-40 ms) if requests arrive split.
	InterFrameDelay time.Duration
}

// interframeDelay returns the Modbus RTU silent interval t3.5 that ends a frame: 3.5
// characters of 11 bits, about 4 ms at 9600 baud. Above 19200 baud the specification fixes
// it at 1.75 ms.
func interframeDelay(baudRate int) time.Duration {
	if baudRate <= 0 {
		baudRate = 9600
	}
	if baudRate <= 19200 {
		return time.Duration(38_500_000_000 / int64(baudRate)) // 3.5 * 11 bits / baud, in ns
	}
	return 1750 * time.Microsecond
}

// defaultReopenInterval is the pause between two attempts to reopen a failed serial port.
const defaultReopenInterval = 5 * time.Second

// SerialStatus is the state of a serial port the server listens on.
type SerialStatus struct {
	Port      string    // device name, e.g. /dev/ttyUSB0
	Connected bool      // the port is open and requests are served
	Err       error     // why the port is not connected; nil while connected
	Since     time.Time // when the state last changed
}

// serialPort tracks one serial port across reopen attempts.
type serialPort struct {
	mu     sync.Mutex
	status SerialStatus
	rwc    io.ReadWriteCloser // current open port, nil while disconnected
	closed bool               // set by Close: no port may be opened any more
}

// connect records rwc as the open port. It reports false, and the caller must close rwc,
// if the server has been closed in the meantime.
func (p *serialPort) connect(rwc io.ReadWriteCloser) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return false
	}
	p.rwc = rwc
	p.status = SerialStatus{Port: p.status.Port, Connected: true, Since: time.Now()}
	return true
}

// fail records err and closes the current port, if any.
func (p *serialPort) fail(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.rwc != nil {
		_ = p.rwc.Close()
		p.rwc = nil
	}
	if p.status.Connected || p.status.Err == nil || p.status.Err.Error() != err.Error() {
		p.status = SerialStatus{Port: p.status.Port, Err: err, Since: time.Now()}
	}
}

func (p *serialPort) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	if p.rwc != nil {
		_ = p.rwc.Close()
		p.rwc = nil
	}
}

func (p *serialPort) current() io.ReadWriteCloser {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.rwc
}

// openSerialPort opens a serial port in blocking mode and wraps it in a frame reader that
// detects the end of an RTU request by the t3.5 inter-frame delay.
func openSerialPort(port string, config SerialConfig) (io.ReadWriteCloser, error) {
	p, err := serial.Open(port, &serial.Mode{
		BaudRate: config.BaudRate,
		DataBits: config.DataBits,
		StopBits: serial.StopBits(config.StopBits),
		Parity:   serial.Parity(config.Parity),
	})
	if err != nil {
		return nil, err
	}
	if err = p.SetReadTimeout(serial.NoTimeout); err != nil {
		_ = p.Close()
		return nil, err
	}

	t := config.Timeout
	if t <= 0 {
		t = time.Second * 5
	}
	delay := config.InterFrameDelay
	if delay <= 0 {
		delay = interframeDelay(config.BaudRate)
	}
	return framereader.NewReadWriteCloser(p, t, delay), nil
}

// ListenRTU starts serving Modbus RTU requests arriving on a serial port, e.g.
//
//	s.ListenRTU(ctx, "/dev/ttyUSB0", mbserver.SerialConfig{BaudRate: 9600, DataBits: 8,
//		StopBits: mbserver.OneStopBit, Parity: mbserver.NoParity})
//
// It returns an error if the port cannot be opened. If the port fails later, e.g. because a
// USB adapter is unplugged, the server reopens it every few seconds until Close; SerialStatus
// reports the state meanwhile.
func (s *Server) ListenRTU(ctx context.Context, port string, config SerialConfig) error {
	rwc, err := s.openSerial(port, config)
	if err != nil {
		return err
	}
	s.serveSerial(ctx, port, rwc, func() (io.ReadWriteCloser, error) { return s.openSerial(port, config) })
	return nil
}

// listenRTUFromReadWriter serves RTU frames from an existing io.ReadWriteCloser, without
// reopening it after a failure. Used for testing.
func (s *Server) listenRTUFromReadWriter(ctx context.Context, rwc io.ReadWriteCloser) {
	s.serveSerial(ctx, "", rwc, nil)
}

// SerialStatus returns the state of every serial port the server listens on.
func (s *Server) SerialStatus() []SerialStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := make([]SerialStatus, 0, len(s.serialPorts))
	for _, p := range s.serialPorts {
		p.mu.Lock()
		status = append(status, p.status)
		p.mu.Unlock()
	}
	return status
}

// serveSerial registers the port and serves it in a goroutine. reopen is nil for a port
// that is not reopened after a failure.
func (s *Server) serveSerial(ctx context.Context, name string, rwc io.ReadWriteCloser, reopen func() (io.ReadWriteCloser, error)) {
	p := &serialPort{status: SerialStatus{Port: name}}
	p.connect(rwc)

	s.mu.Lock()
	s.serialPorts = append(s.serialPorts, p)
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			err := s.acceptSerialRequests(ctx, p.current())
			if err == nil || s.stopping(ctx) {
				return
			}
			s.log.Error("Serial port failed", "port", name, "error", err)
			p.fail(err)
			if reopen == nil {
				return
			}
			if !s.reopenSerial(ctx, p, reopen) {
				return
			}
			s.log.Info("Serial port reopened", "port", name)
		}
	}()
}

// reopenSerial tries to reopen the port every reopenInterval until it succeeds (true) or the
// server stops (false).
func (s *Server) reopenSerial(ctx context.Context, p *serialPort, reopen func() (io.ReadWriteCloser, error)) bool {
	for {
		select {
		case <-ctx.Done():
			return false
		case <-s.done:
			return false
		case <-time.After(s.reopenInterval):
		}

		rwc, err := reopen()
		if err != nil {
			s.log.Debug("Serial port still unavailable", "port", p.status.Port, "error", err)
			p.fail(err)
			continue
		}
		if !p.connect(rwc) {
			_ = rwc.Close()
			return false
		}
		return true
	}
}

// stopping reports whether ctx is done or the server is closed.
func (s *Server) stopping(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	case <-s.done:
		return true
	default:
		return false
	}
}

// acceptSerialRequests serves requests from reader until the server stops (nil) or reading
// fails (the error).
func (s *Server) acceptSerialRequests(ctx context.Context, reader io.ReadWriteCloser) error {
	if reader == nil {
		return errors.New("serial port not open")
	}
	for {
		if s.stopping(ctx) {
			return nil
		}

		frame := make([]byte, 512)
		n, err := reader.Read(frame)
		if errors.Is(err, framereader.ErrTimeout) {
			// No request within the timeout, e.g. a client that polls rarely or not at
			// night: keep listening.
			continue
		}
		if err != nil {
			if s.stopping(ctx) {
				return nil
			}
			if errors.Is(err, io.EOF) {
				err = errors.New("serial port closed unexpectedly")
			}
			return err
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
			s.log.Warn("Error parsing RTU frame", "error", err)
			continue
		}
		select {
		case s.request <- Request{reader, rtuFrame}:
		case <-ctx.Done():
			return nil
		case <-s.done:
			return nil
		}
	}
}
