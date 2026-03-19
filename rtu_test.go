package mbserver

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"go.bug.st/serial"
)

func TestSerialHW(t *testing.T) {
	if _, err := serial.Open("/dev/ttyUSB0", &serial.Mode{
		BaudRate: 9600,
		DataBits: 8,
		StopBits: serial.OneStopBit,
		Parity:   serial.NoParity,
	}); err != nil {
		t.Skip("hardware not available:", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := NewServer(slog.Default())
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.ListenRTU(ctx, "/dev/ttyUSB0", SerialConfig{
		BaudRate: 9600,
		DataBits: 8,
		StopBits: OneStopBit,
		Parity:   NoParity,
		Timeout:  5 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}
}
