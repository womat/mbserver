// Package mbserver implements a Modbus server for TCP and RTU with several unit IDs.
package mbserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"slices"
	"sync"
	"sync/atomic"
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
	units          map[byte]Unit
	offline        map[byte]bool // unit IDs that keep their registers but do not answer, see SetOnline
	noBroadcast    bool          // unit ID 0 is dropped instead of applied to every unit, see SetBroadcast

	connSemaphore chan struct{} // limits concurrent connections

	startOnce sync.Once
	cancel    context.CancelFunc // stops the handler goroutine on Close()
	wg        sync.WaitGroup     // tracks active goroutines for clean shutdown
	request   chan Request

	done      chan struct{} // closed by Close; stops listeners, connections and serial readers
	closeOnce sync.Once

	tcpRequests atomic.Uint64 // requests answered over TCP, see Stats
	rtuRequests atomic.Uint64 // requests answered over RTU, see Stats

	clientsMu sync.Mutex
	clients   map[io.ReadWriteCloser]*Client // open TCP connections, see Clients
}

// Client is an open Modbus TCP connection, see Clients.
type Client struct {
	Remote   net.Addr  // address of the client
	Since    time.Time // when the connection was accepted
	Requests uint64    // requests answered on this connection, exceptions included
}

// Request contains the connection and Modbus frame.
type Request struct {
	conn  io.ReadWriteCloser
	frame Framer
}

// Unit contains the registers of one Modbus unit ID.
type Unit struct {
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

		clients: map[io.ReadWriteCloser]*Client{},
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
	s.units = map[byte]Unit{}
	s.offline = map[byte]bool{}
	if err := s.NewUnit(1); err != nil {
		panic("mbserver: failed to create default unit: " + err.Error())
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

func (s *Server) NewUnit(id byte) error {
	if id < idMin || id > idMax {
		return fmt.Errorf("invalid modbus id %v", id)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.units[id]; ok {
		return fmt.Errorf("mbserver: unit %v already exists", id)
	}
	s.units[id] = Unit{
		DiscreteInputs:   make([]byte, 65536),
		Coils:            make([]byte, 65536),
		HoldingRegisters: make([]uint16, 65536),
		InputRegisters:   make([]uint16, 65536),
	}

	return nil
}

func (s *Server) RemoveUnit(id byte) error {
	if id < idMin || id > idMax {
		return fmt.Errorf("invalid modbus id %v", id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.units[id]; !ok {
		return fmt.Errorf("mbserver: unit %v doesn't exist", id)
	}
	// delete Modbus memory maps.
	delete(s.units, id)
	delete(s.offline, id)
	return nil
}

// ErrRange is returned when a register range does not fit into the 65536 registers of a unit.
var ErrRange = errors.New("mbserver: register range out of bounds")

// SetHoldingRegisters writes values to the holding registers of unit id, starting at
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

// HoldingRegisters returns a copy of quantity holding registers of unit id, starting at start.
func (s *Server) HoldingRegisters(id byte, start uint16, quantity int) ([]uint16, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dev, ok := s.units[id]
	if !ok {
		return nil, fmt.Errorf("mbserver: unit %v doesn't exist", id)
	}
	if quantity < 0 || int(start)+quantity > len(dev.HoldingRegisters) {
		return nil, ErrRange
	}
	return append([]uint16(nil), dev.HoldingRegisters[int(start):int(start)+quantity]...), nil
}

// UpdateHoldingRegisters calls update with the holding registers of unit id while holding
// the write lock, so several registers can be changed as one consistent update. update must
// not keep the slice or call other methods of s.
func (s *Server) UpdateHoldingRegisters(id byte, update func(registers []uint16) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dev, ok := s.units[id]
	if !ok {
		return fmt.Errorf("mbserver: unit %v doesn't exist", id)
	}
	return update(dev.HoldingRegisters)
}

// SetOnline takes a unit ID off the bus (online false) or back on it (online true). An offline
// unit is answered like an unknown one - silence over RTU, exception GatewayTargetDeviceFailedToRespond
// over TCP - but keeps its registers, and SetHoldingRegisters and UpdateHoldingRegisters still
// work. So a gateway can go silent while its source is lost, and come back with fresh values
// without a moment of empty registers.
func (s *Server) SetOnline(id byte, online bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.units[id]; !ok {
		return fmt.Errorf("mbserver: unit %v doesn't exist", id)
	}
	if online {
		delete(s.offline, id)
	} else {
		s.offline[id] = true
	}
	return nil
}

// Online reports whether the unit ID exists and is online.
func (s *Server) Online(id byte) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.units[id]
	return ok && !s.offline[id]
}

// SetBroadcast controls requests for unit ID 0. Enabled (the default), they are applied to
// every unit ID and not answered, as specified. Disabled, they are dropped: silence over RTU,
// exception GatewayTargetDeviceFailedToRespond over TCP. A gateway whose handlers forward
// requests to real devices disables it, so a broadcast write cannot reach all of them at once.
func (s *Server) SetBroadcast(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.noBroadcast = !enabled
}

// Stats is a snapshot of the server's activity since NewServer.
type Stats struct {
	TCPRequests uint64 // requests answered over TCP, exceptions included
	RTURequests uint64 // requests answered over RTU, exceptions included
	TCPClients  int    // TCP connections open right now
}

// Stats returns the request counters and the number of connected TCP clients. Requests for
// unknown or offline unit IDs and broadcasts are not counted. A client polls the counters and
// takes the difference to show activity.
func (s *Server) Stats() Stats {
	return Stats{
		TCPRequests: s.tcpRequests.Load(),
		RTURequests: s.rtuRequests.Load(),
		TCPClients:  len(s.connSemaphore),
	}
}

// Clients returns the open Modbus TCP connections, the oldest first. Like Stats, Requests counts
// the answered requests; requests for unknown or offline unit IDs and broadcasts are not counted.
func (s *Server) Clients() []Client {
	s.clientsMu.Lock()
	list := make([]Client, 0, len(s.clients))
	for _, c := range s.clients {
		list = append(list, *c)
	}
	s.clientsMu.Unlock()
	slices.SortFunc(list, func(a, b Client) int { return a.Since.Compare(b.Since) })
	return list
}

func (s *Server) addClient(conn net.Conn) {
	s.clientsMu.Lock()
	s.clients[conn] = &Client{Remote: conn.RemoteAddr(), Since: time.Now()}
	s.clientsMu.Unlock()
}

func (s *Server) removeClient(conn net.Conn) {
	s.clientsMu.Lock()
	delete(s.clients, conn)
	s.clientsMu.Unlock()
}

func (s *Server) countClient(conn io.ReadWriteCloser) {
	s.clientsMu.Lock()
	if c := s.clients[conn]; c != nil {
		c.Requests++
	}
	s.clientsMu.Unlock()
}

// RegisterFunctionHandler overrides the handler for a Modbus function code; nil removes it, so the
// function is answered with IllegalFunction.
func (s *Server) RegisterFunctionHandler(funcCode uint8, function func(*Server, Framer) ([]byte, Exception)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.function[funcCode] = function
}

func (s *Server) handle(request Request) (response Framer) {
	var exception Exception
	var data []byte

	response = request.frame.Copy()
	function := request.frame.GetFunction()

	// A panicking handler, e.g. a custom one indexing a short frame, must not take the whole
	// process down: answer ServerDeviceFailure and keep serving.
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("function handler panicked", "function", function, "unitId", request.frame.GetUnitId(), "panic", r)
			response = request.frame.Copy()
			response.SetException(ServerDeviceFailure)
		}
	}()

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
			unitId := request.frame.GetUnitId()
			_, isTCP := request.frame.(*TCPFrame)
			if unitId == 0 {
				s.mu.RLock() // only to read the unit map
				noBroadcast := s.noBroadcast
				units := make([]byte, 0, len(s.units))
				for id := range s.units {
					units = append(units, id)
				}
				s.mu.RUnlock()

				if noBroadcast {
					s.log.Debug("broadcast dropped")
					s.reject(request, isTCP)
					continue
				}
				for _, id := range units { // iterate without the lock
					request.frame.SetUnitId(id)
					_ = s.handle(request) // the handlers take the lock themselves
				}
				continue
			}

			s.mu.RLock()
			_, ok := s.units[unitId]
			ok = ok && !s.offline[unitId]
			s.mu.RUnlock()

			if !ok {
				s.log.Debug("request for an unknown or offline unit ID", "unitId", unitId)
				s.reject(request, isTCP)
				continue
			}

			response := s.handle(request) // the handlers take the lock themselves
			if _, err := request.conn.Write(response.Bytes()); err != nil {
				s.log.Warn("failed to write response", "error", err)
				continue
			}
			if isTCP {
				s.tcpRequests.Add(1)
				s.countClient(request.conn)
			} else {
				s.rtuRequests.Add(1)
			}
		}
	}
}

// reject answers a request nobody serves. On a serial bus another device may own the unit ID,
// so it stays silent. Over TCP there is nobody else to answer: it reports exception
// GatewayTargetDeviceFailedToRespond, so the client does not run into its timeout.
func (s *Server) reject(request Request, isTCP bool) {
	if !isTCP {
		return
	}
	response := request.frame.Copy()
	response.SetException(GatewayTargetDeviceFailedToRespond)
	if _, err := request.conn.Write(response.Bytes()); err != nil {
		s.log.Warn("failed to write response", "error", err)
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
