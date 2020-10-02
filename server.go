// Package mbserver implments a Modbus server (slave).
package mbserver

import (
	"encoding/hex"
	"fmt"
	"io"
	"net"
)

// Server is a Modbus slave with allocated memory for discrete inputs, coils, etc.
type Server struct {
	// Debug enables more verbose messaging.
	Debug       bool
	listeners   []net.Listener
	ports       []io.ReadWriteCloser
	requestChan chan *Request
	function    [256]func(*Server, Framer) ([]byte, Exception)
	Devices     map[byte]Device
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
	s.Devices = map[byte]Device{}
	_ = s.NewDevice(1)

	s.requestChan = make(chan *Request)
	go s.handler()

	return s
}

// TODO >> sollte einen Pointer zu den Registern zurückgeben
// TODO Sollte auch nur New heißen
func (s *Server) NewDevice(id byte) error {
	if id < idmin && id > idmax {
		return fmt.Errorf("invalid modbus id %v", id)
	}
	if _, ok := s.Devices[id]; ok {
		return fmt.Errorf("mbserver: device %v already exists", id)
	}
	s.Devices[id] = Device{
		DiscreteInputs:   make([]byte, 65536),
		Coils:            make([]byte, 65536),
		HoldingRegisters: make([]uint16, 65536),
		InputRegisters:   make([]uint16, 65536),
	}

	return nil
}

// TODO Sollte auch nur Close heißen
func (s *Server) RemoveDevice(id byte) error {
	if id < idmin && id > idmax {
		return fmt.Errorf("invalid modbus id %v", id)
	}
	if _, ok := s.Devices[id]; !ok {
		return fmt.Errorf("mbserver: device %v doesn't exists", id)
	}
	// delete Modbus memory maps.
	delete(s.Devices, id)
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
		infolog.Printf("IllegalFunction: %v\n", function)
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
		if device == 0 {
			debuglog.Printf("start modbus broadcast")
			for device, _ := range s.Devices {
				request.frame.SetDevice(device)
				_ = s.handle(request)
				//  Broadcast doesn't send response
			}
			debuglog.Printf("end modbus broadcast:")
		} else {
			if _, ok := s.Devices[device]; !ok {
				//  ignore request if  device is unknown
				debuglog.Printf("unknown deviceid: %v\n", device)
				return
			}
			//	if response, exception := s.handle(request); exception != &InternalError {
			response := s.handle(request)
			r := response.Bytes()
			tracelog.Printf("write serial port: %v", hex.EncodeToString(r))
			request.conn.Write(r)
			//	}
		}
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
