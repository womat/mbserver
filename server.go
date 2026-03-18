// Package mbserver implements a Modbus server (slave).
package mbserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
)

var (
	ErrAlreadyStarted = errors.New("server already started")
)

const (
	// defaultMaxConnections limits concurrent clients to prevent resource exhaustion.
	defaultMaxConnections = 100
)

// Server is a Modbus slave with allocated memory for discrete inputs, coils, etc.
type Server struct {
	// Debug enables more verbose messaging.
	mu        sync.RWMutex
	log       *slog.Logger
	listeners []net.Listener

	ports    []io.ReadWriteCloser
	function [256]func(*Server, Framer) ([]byte, Exception)
	devices  map[byte]Device

	connSemaphore chan struct{} // limits concurrent connections

	startOnce sync.Once
	wg        sync.WaitGroup // tracks active goroutines for clean shutdown
	request   chan Request
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

// NewServer creates a new Modbus server (slave).
func NewServer(log *slog.Logger) *Server {
	s := &Server{
		log:           log,
		request:       make(chan Request),
		connSemaphore: make(chan struct{}, defaultMaxConnections),
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
	_ = s.NewDevice(1)
	return s
}

func (s *Server) Start(ctx context.Context) error {

	err := ErrAlreadyStarted
	s.startOnce.Do(func() {
		err = nil
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

// TODO Sollte auch nur Close heißen
func (s *Server) RemoveDevice(id byte) error {
	if id < idMin || id > idMax {
		return fmt.Errorf("invalid modbus id %v", id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devices[id]; !ok {
		return fmt.Errorf("mbserver: device %v doesn't exists", id)
	}
	// delete Modbus memory maps.
	delete(s.devices, id)
	return nil
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
		s.log.Error("illegal Function", function)
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
			// s.mu.RLock()    ← weg
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
				s.log.Warn("modbus device doesn't exists", "device", device)
				continue
			}

			response := s.handle(request) // handle() → ReadCoils etc. → eigener Lock
			if _, err := request.conn.Write(response.Bytes()); err != nil {
				s.log.Warn("failed to write response", "error", err)
			}
		}
	}
}

// Close stops listening to TCP/IP ports and closes serial ports.
func (s *Server) Close() {
	s.mu.Lock()

	for _, listen := range s.listeners {
		_ = listen.Close()
	}
	for _, port := range s.ports {
		_ = port.Close()
	}
	s.mu.Unlock()

	s.wg.Wait()
}
