package mbserver

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/womat/mbserver/pkg/framereader"
)

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// TestRTUKeepsServingAfterIdle: an RTU client that is silent longer than the read timeout
// (e.g. an inverter at night) must still get answers afterwards.
func TestRTUKeepsServingAfterIdle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := NewServer(quietLogger())
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	client, port := net.Pipe()
	defer client.Close()
	s.listenRTUFromReadWriter(ctx, framereader.NewReadWriteCloser(port, 100*time.Millisecond, 5*time.Millisecond))

	time.Sleep(300 * time.Millisecond) // three read timeouts without a request

	request := RTUFrame{Address: 1, Function: 3}
	SetRegisterValues(&request, 0, 1, []uint16{})
	_ = client.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := client.Write(request.Bytes()); err != nil {
		t.Fatalf("write request: %v", err)
	}
	response := make([]byte, 16)
	n, err := client.Read(response)
	if err != nil || n != 7 {
		t.Fatalf("no answer after an idle period: n=%d err=%v", n, err)
	}
}

// TestCloseWithConnectedTCPClient: Close must not wait for clients to disconnect, and a
// connected client must not be served by the closed server any longer.
func TestCloseWithConnectedTCPClient(t *testing.T) {
	s := NewServer(quietLogger())
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := l.Addr().String()
	_ = l.Close()
	if err = s.ListenTCP(context.Background(), address); err != nil {
		t.Fatal(err)
	}

	conn, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	request := TCPFrame{TransactionIdentifier: 1, Device: 1, Function: 3}
	SetRegisterValues(&request, 0, 1, []uint16{})
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err = conn.Write(request.Bytes()); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Read(make([]byte, 32)); err != nil {
		t.Fatalf("no answer before Close: %v", err)
	}

	closed := make(chan struct{})
	go func() { s.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Close blocks while a TCP client is connected")
	}

	_, _ = conn.Write(request.Bytes())
	if _, err = conn.Read(make([]byte, 32)); err == nil {
		t.Error("the closed server still answers an existing connection")
	}
}

func TestHoldingRegisterAPI(t *testing.T) {
	s := NewServer(quietLogger())
	if err := s.NewDevice(200); err != nil {
		t.Fatal(err)
	}

	if err := s.SetHoldingRegisters(200, 4124, []uint16{0x03F6, 0xFAFA}); err != nil {
		t.Fatal(err)
	}
	got, err := s.HoldingRegisters(200, 4124, 2)
	if err != nil || got[0] != 0x03F6 || got[1] != 0xFAFA {
		t.Errorf("HoldingRegisters() = %v, %v; want [0x3f6 0xfafa]", got, err)
	}
	if other, _ := s.HoldingRegisters(1, 4124, 1); other[0] != 0 {
		t.Errorf("device 1 register 4124 = %d, want 0: devices must not share registers", other[0])
	}

	if err = s.SetHoldingRegisters(200, 65535, []uint16{1, 2}); err != ErrRange {
		t.Errorf("SetHoldingRegisters past the end = %v, want ErrRange", err)
	}
	if _, err = s.HoldingRegisters(7, 0, 1); err == nil {
		t.Error("HoldingRegisters on an unknown device succeeded")
	}

	err = s.UpdateHoldingRegisters(1, func(r []uint16) error { r[0], r[1] = 7, 8; return nil })
	if got, _ = s.HoldingRegisters(1, 0, 2); err != nil || got[0] != 7 || got[1] != 8 {
		t.Errorf("UpdateHoldingRegisters() wrote %v, %v; want [7 8]", got, err)
	}
}

// TestNoTornReads updates a 32-bit value continuously while a TCP client reads it; both words
// always carry the same counter, so a read mixing an old and a new word is detected.
func TestNoTornReads(t *testing.T) {
	s := NewServer(quietLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := l.Addr().String()
	_ = l.Close()
	if err = s.ListenTCP(ctx, address); err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	stop := make(chan struct{})
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for i := uint16(0); ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			_ = s.SetHoldingRegisters(1, 4124, []uint16{i, i})
		}
	}()

	request := TCPFrame{TransactionIdentifier: 1, Device: 1, Function: 3}
	SetRegisterValues(&request, 4124, 2, []uint16{})
	response := make([]byte, 13) // MBAP 7 + function 1 + byte count 1 + 4 data
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	for range 300 {
		if _, err = conn.Write(request.Bytes()); err != nil {
			t.Fatal(err)
		}
		if _, err = io.ReadFull(conn, response); err != nil {
			t.Fatal(err)
		}
		if hi, lo := response[9:11], response[11:13]; hi[0] != lo[0] || hi[1] != lo[1] {
			t.Fatalf("torn read: high word % x, low word % x", hi, lo)
		}
	}
	close(stop)
	<-writerDone
}
