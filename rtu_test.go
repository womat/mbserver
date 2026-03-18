package mbserver

import (
	"testing"
	"time"

	"go.bug.st/serial"
)

// in server_test.go oder rtu_test.go

type mockSerialPort struct {
	readData []byte
	written  []byte
	timeout  time.Duration
}

func (m *mockSerialPort) Read(buf []byte) (int, error) {
	if len(m.readData) == 0 {
		time.Sleep(m.timeout) // simuliert Timeout
		return 0, nil
	}
	n := copy(buf, m.readData)
	m.readData = m.readData[n:]
	return n, nil
}

func (m *mockSerialPort) Write(buf []byte) (int, error) {
	m.written = append(m.written, buf...)
	return len(buf), nil
}

func (m *mockSerialPort) Close() error { return nil }

func (m *mockSerialPort) SetReadTimeout(t time.Duration) error {
	m.timeout = t
	return nil
}

func TestSerialHW(t *testing.T) {

	port, _ := serial.Open("/dev/ttyUSB0", &serial.Mode{
		BaudRate: 9600,
		DataBits: 8,
		StopBits: serial.OneStopBit,
		Parity:   serial.NoParity,
	})

	_ = NewServer().ListenRTU(port, 9600)
}
