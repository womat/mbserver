// Package mbserver implements a Modbus server (slave).
package mbserver

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
)

// Server is a Modbus slave with allocated memory for discrete inputs, coils, etc.
type Server struct {
	// Debug enables more verbose messaging.
	mu          sync.RWMutex
	log         slog.Logger
	Debug       bool
	listeners   []net.Listener
	ports       []io.ReadWriteCloser
	requestChan chan *Request
	function    [256]func(*Server, Framer) ([]byte, Exception)
	devices     map[byte]Device
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

// TODO Sollte auch nur New heißen
// NewServer creates a new Modbus server (slave).
func NewServer() *Server {
	s := &Server{}

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

	s.requestChan = make(chan *Request)
	go s.handler()

	return s
}

// TODO >> sollte einen Pointer zu den Registern zurückgeben
// TODO Sollte auch nur New heißen
func (s *Server) NewDevice(id byte) error {
	if id < idmin || id > idmax {
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
	if id < idmin || id > idmax {
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
	s.function[funcCode] = function
}

func (s *Server) handle(request *Request) Framer {
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
func (s *Server) handler() {
	for {
		request := <-s.requestChan
		device := request.frame.GetDevice()
		s.mu.RLock()
		if device == 0 {
			s.log.Info("starting modbus broadcast")
			for device, _ := range s.devices {
				request.frame.SetDevice(device)
				_ = s.handle(request)
				//  Broadcast doesn't send response!!
			}

			s.mu.RUnlock()

			s.log.Info("modbus broadcast finished")
			continue
		}
		if _, ok := s.devices[device]; !ok {
			//  ignore request if device is unknown
			s.mu.RUnlock()

			s.log.Warn("modbus device doesn't exists", "device", device)
			continue
		}
		response := s.handle(request)
		r := response.Bytes()

		s.log.Debug("modbus broadcast received", "request", request, "response", response)
		s.mu.RUnlock()

		request.conn.Write(r)

	}
}

// Close stops listening to TCP/IP ports and closes serial ports.
func (s *Server) Close() {
	for _, listen := range s.listeners {
		listen.Close()
	}
	for _, port := range s.ports {
		port.Close()
	}
}
