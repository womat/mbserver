//go:build linux
// +build linux

package mbserver

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/goburrow/modbus"
)

// TestModbusRTU serves RTU over a pair of virtual serial ports (socat ptys), through the
// real serial and frame reader stack. It is skipped where socat is not installed.
func TestModbusRTU(t *testing.T) {
	if _, err := exec.LookPath("socat"); err != nil {
		t.Skip("socat not installed")
	}

	// Create a pair of virtual serial units.
	dir := t.TempDir()
	foo, bar := filepath.Join(dir, "ttyFOO"), filepath.Join(dir, "ttyBAR")
	cmd := exec.Command("socat", "pty,raw,echo=0,link="+foo, "pty,raw,echo=0,link="+bar)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Wait()
	defer cmd.Process.Kill()

	// Wait for the virtual serial units to be created.
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, errFoo := os.Stat(foo)
		_, errBar := os.Stat(bar)
		if errFoo == nil && errBar == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("socat did not create the virtual serial ports")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Server
	s := NewServer(slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Start(ctx); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	err := s.ListenRTU(ctx, foo, SerialConfig{
		BaudRate: 115200,
		DataBits: 8,
		StopBits: OneStopBit,
		Parity:   NoParity,
		Timeout:  10 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to listen, got %v", err)
	}
	defer s.Close()

	// Allow the server to start and to avoid a connection refused on the client
	time.Sleep(1 * time.Millisecond)

	// Client
	handler := modbus.NewRTUClientHandler(bar)
	handler.BaudRate = 115200
	handler.DataBits = 8
	handler.Parity = "N"
	handler.StopBits = 1
	handler.SlaveId = 1
	handler.Timeout = 5 * time.Second
	// Connect manually so that multiple requests are handled in one connection session
	err = handler.Connect()
	if err != nil {
		t.Errorf("failed to connect, got %v\n", err)
		t.FailNow()
	}
	defer handler.Close()
	client := modbus.NewClient(handler)

	// Coils
	_, err = client.WriteMultipleCoils(100, 9, []byte{255, 1})
	if err != nil {
		t.Errorf("expected nil, got %v\n", err)
		t.FailNow()
	}

	results, err := client.ReadCoils(100, 16)
	if err != nil {
		t.Errorf("expected nil, got %v\n", err)
		t.FailNow()
	}
	expect := []byte{255, 1}
	got := results
	if !isEqual(expect, got) {
		t.Errorf("expected %v, got %v", expect, got)
	}
}
