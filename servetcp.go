package mbserver

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
)

const (
	// mbapHeaderSize is the fixed size of the Modbus TCP MBAP header:
	// Transaction ID (2) + Protocol ID (2) + Length (2) + Unit ID (1) = 7 bytes
	mbapHeaderSize = 7

	// maxPacketSize is the maximum allowed Modbus TCP ADU size (header + 253 byte PDU).
	maxPacketSize = mbapHeaderSize + 253
)

// ListenTCP starts the Modbus server listening on "address:port".
func (s *Server) ListenTCP(ctx context.Context, addressPort string) (err error) {
	listen, err := net.Listen("tcp", addressPort)
	if err != nil {
		s.log.Error("Unable to listen", "error", err)
		return err
	}
	s.mu.Lock()
	s.listeners = append(s.listeners, listen)
	s.mu.Unlock()

	s.wg.Add(1)
	go s.acceptTCP(ctx, listen)
	return nil
}

func (s *Server) acceptTCP(ctx context.Context, listen net.Listener) {
	defer s.wg.Done()

	// Close the listener when ctx is cancelled so Accept() unblocks.
	go func() {
		<-ctx.Done()
		_ = listen.Close()
	}()

	for {
		conn, err := listen.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			s.log.Warn("failed to accept client connection", "error", err)
			continue
		}

		// Acquire the connection semaphore; drop the connection if we are at capacity.
		select {
		case s.connSemaphore <- struct{}{}:
		default:
			s.log.Warn("Max connections reached, rejecting client", "remote", conn.RemoteAddr())
			_ = conn.Close()
			continue
		}

		s.wg.Add(1)
		go s.handleConn(ctx, conn)
	}
}

// handleConn reads a stream of Modbus TCP frames from conn and forwards each
// Request to the processing channel.
func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer func() {
		_ = conn.Close()
		<-s.connSemaphore // release the slot
		s.wg.Done()
	}()

	remote := conn.RemoteAddr()

	for {
		// --- 1. Read the fixed-size MBAP header first ---
		header := make([]byte, mbapHeaderSize)
		if _, err := io.ReadFull(conn, header); err != nil {
			if err != io.EOF && !errors.Is(err, net.ErrClosed) {
				s.log.Warn("Unable to read MBAP header", "remote", remote, "error", err)
			}
			return
		}

		// --- 2. Decode the MBAP header ---
		// The Length field covers Unit ID (1 byte) + PDU (n bytes).
		// txnId := binary.BigEndian.Uint16(header[0:2])
		// protocolId := binary.BigEndian.Uint16(header[2:4])
		length := binary.BigEndian.Uint16(header[4:6])
		// unitId := header[6]

		if length < 1 {
			s.log.Warn("Invalid MBAP length", "remote", remote, "length", length)
			return
		}
		pduLen := int(length) - 1 // subtract the Unit ID byte already in header

		totalLen := mbapHeaderSize + pduLen
		if totalLen > maxPacketSize {
			s.log.Warn("Packet too large", "remote", remote, "size", totalLen)
			return
		}

		// --- 3. Read the PDU body ---
		packet := make([]byte, totalLen)
		copy(packet, header)
		if pduLen > 0 {
			if _, err := io.ReadFull(conn, packet[mbapHeaderSize:]); err != nil {
				s.log.Warn("Unable to read PDU body", "remote", remote, "error", err)
				return
			}
		}

		// --- 4. Parse and dispatch ---
		frame, err := NewTCPFrame(packet)
		if err != nil {
			s.log.Warn("Unable to parse frame", "remote", remote, "error", err)
			return
		}

		select {
		case s.request <- Request{conn, frame}:
		case <-ctx.Done():
			return
		}
	}
}
