// Package mbserver implements a Modbus server for TCP and RTU with several unit IDs.
package mbserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"
)

var (
	ErrAlreadyStarted = errors.New("server already started")
)

const (
	// defaultMaxConnections limits concurrent clients to prevent resource exhaustion.
	defaultMaxConnections = 100
)

// Server is a Modbus server with allocated memory for discrete inputs, coils, etc. per unit ID.
type Server struct {
	mu        sync.RWMutex
	log       *slog.Logger
	listeners []net.Listener

	serialPorts    []*serialPort
	openSerial     func(port string, config SerialConfig) (io.ReadWriteCloser, error)
	reopenInterval time.Duration
	function       [256]func(*Server, Framer) ([]byte, Exception)
	devices        map[byte]Device

	connSemaphore chan struct{} // limits concurrent connections

	startOnce sync.Once
	cancel    context.CancelFunc // stops the handler goroutine on Close()
	wg        sync.WaitGroup     // tracks active goroutines for clean shutdown
	request   chan Request

	done      chan struct{} // closed by Close; stops listeners, connections and serial readers
	closeOnce sync.Once
}

// Request contains the connection and Modbus frame.
type Request struct {
	conn  io.ReadWriteCloser
	frame Framer
}

// Device contains the Registers of a Modbus Device.
type Device struct {
	DiscreteInputs   []byte
	Coils            []byte
	HoldingRegisters []uint16
	InputRegisters   []uint16
}

// NewServer creates a new Modbus server; unit ID 1 exists from the start. Call Start before
// listening, and Close to stop it.
func NewServer(log *slog.Logger) *Server {
	s := &Server{
		log:           log,
		request:       make(chan Request),
		connSemaphore: make(chan struct{}, defaultMaxConnections),
		done:          make(chan struct{}),

		openSerial:     openSerialPort,
		reopenInterval: defaultReopenInterval,
	}

	// Add default functions.
	s.function[1] = ReadCoils
	s.function[2] = ReadDiscreteInputs
	s.function[3] = ReadHoldingRegisters
	s.function[4] = ReadInputRegisters
	s.function[5] = WriteSingleCoil
	s.function[6] = WriteHoldingRegister
	s.function[15] = WriteMultipleCoils
	s.function[16] = WriteHoldingRegisters

	// Allocate Modbus memory maps.
	s.devices = map[byte]Device{}
	if err := s.NewDevice(1); err != nil {
		panic("mbserver: failed to create default device: " + err.Error())
	}
	return s
}

func (s *Server) Start(ctx context.Context) error {
	err := ErrAlreadyStarted
	s.startOnce.Do(func() {
		err = nil
		ctx, s.cancel = context.WithCancel(ctx)
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handler(ctx)
		}()
	})
	return err
}

func (s *Server) NewDevice(id byte) error {
	if id < idMin || id > idMax {
		return fmt.Errorf("invalid modbus id %v", id)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.devices[id]; ok {
		return fmt.Errorf("mbserver: device %v already exists", id)
	}
	s.devices[id] = Device{
		DiscreteInputs:   make([]byte, 65536),
		Coils:            make([]byte, 65536),
		HoldingRegisters: make([]uint16, 65536),
		InputRegisters:   make([]uint16, 65536),
	}

	return nil
}

func (s *Server) RemoveDevice(id byte) error {
	if id < idMin || id > idMax {
		return fmt.Errorf("invalid modbus id %v", id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devices[id]; !ok {
		return fmt.Errorf("mbserver: device %v doesn't exist", id)
	}
	// delete Modbus memory maps.
	delete(s.devices, id)
	return nil
}

// ErrRange is returned when a register range does not fit into the 65536 registers of a device.
var ErrRange = errors.New("mbserver: register range out of bounds")

// SetHoldingRegisters writes values to the holding registers of device id, starting at
// start, under one lock: a client never reads a partly written multi-register value.
func (s *Server) SetHoldingRegisters(id byte, start uint16, values []uint16) error {
	return s.UpdateHoldingRegisters(id, func(registers []uint16) error {
		if int(start)+len(values) > len(registers) {
			return ErrRange
		}
		copy(registers[start:], values)
		return nil
	})
}

// HoldingRegisters returns a copy of quantity holding registers of device id, starting at start.
func (s *Server) HoldingRegisters(id byte, start uint16, quantity int) ([]uint16, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dev, ok := s.devices[id]
	if !ok {
		return nil, fmt.Errorf("mbserver: device %v doesn't exist", id)
	}
	if quantity < 0 || int(start)+quantity > len(dev.HoldingRegisters) {
		return nil, ErrRange
	}
	return append([]uint16(nil), dev.HoldingRegisters[int(start):int(start)+quantity]...), nil
}

// UpdateHoldingRegisters calls update with the holding registers of device id while holding
// the write lock, so several registers can be changed as one consistent update. update must
// not keep the slice or call other methods of s.
func (s *Server) UpdateHoldingRegisters(id byte, update func(registers []uint16) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dev, ok := s.devices[id]
	if !ok {
		return fmt.Errorf("mbserver: device %v doesn't exist", id)
	}
	return update(dev.HoldingRegisters)
}

// RegisterFunctionHandler override the default behavior for a given Modbus function.
func (s *Server) RegisterFunctionHandler(funcCode uint8, function func(*Server, Framer) ([]byte, Exception)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.function[funcCode] = function
}

func (s *Server) handle(request Request) Framer {
	var exception Exception
	var data []byte

	response := request.frame.Copy()
	function := request.frame.GetFunction()

	if s.function[function] != nil {
		data, exception = s.function[function](s, request.frame)
		response.SetData(data)
	} else {
		s.log.Debug("illegal function", "function", function)
		exception = IllegalFunction
	}

	if exception != Success {
		response.SetException(exception)
	}

	return response
}

// All requests are handled synchronously to prevent modbus memory corruption.
func (s *Server) handler(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case request := <-s.request:
			device := request.frame.GetDevice()
			if device == 0 {
				// broadcast
				s.mu.RLock() // nur für das Lesen der device-Map
				devices := make([]byte, 0, len(s.devices))
				for id := range s.devices {
					devices = append(devices, id)
				}
				s.mu.RUnlock()

				for _, id := range devices { // Lock-frei iterieren
					request.frame.SetDevice(id)
					_ = s.handle(request) // handle() holt eigenen Lock
				}
				continue
			}

			s.mu.RLock()
			_, ok := s.devices[device]
			s.mu.RUnlock()

			if !ok {
				// On a serial bus another device may own the unit ID, so stay silent. Over TCP
				// there is nobody else to answer: report it, so the client does not time out.
				if _, isTCP := request.frame.(*TCPFrame); isTCP {
					s.log.Debug("request for an unknown unit ID", "device", device)
					response := request.frame.Copy()
					response.SetException(GatewayTargetDeviceFailedToRespond)
					if _, err := request.conn.Write(response.Bytes()); err != nil {
						s.log.Warn("failed to write response", "error", err)
					}
				}
				continue
			}

			response := s.handle(request) // handle() → ReadCoils etc. → eigener Lock
			if _, err := request.conn.Write(response.Bytes()); err != nil {
				s.log.Warn("failed to write response", "error", err)
			}
		}
	}
}

// Close stops the server: cancels the handler, closes all listeners, client connections and
// serial ports, then waits for all goroutines to finish. Clients still connected are
// disconnected, so they reconnect to whatever serves the address next. Safe to call more
// than once.
func (s *Server) Close() {
	s.closeOnce.Do(func() { close(s.done) })
	if s.cancel != nil {
		s.cancel()
	}

	s.mu.Lock()
	for _, listen := range s.listeners {
		_ = listen.Close()
	}
	for _, port := range s.serialPorts {
		port.close()
	}
	s.mu.Unlock()

	s.wg.Wait()
}
