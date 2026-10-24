package mbserver

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

// expectClosed reports whether the server closes conn within two seconds without answering.
func expectClosed(t *testing.T, conn net.Conn, what string) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(make([]byte, 32))
	if n != 0 || !errors.Is(err, io.EOF) {
		t.Errorf("%s: got %d bytes and %v, want the connection closed", what, n, err)
	}
}

// TestMalformedMBAPClosesConnection: a request with a broken MBAP header closes its
// connection, and the server keeps serving the others.
func TestMalformedMBAPClosesConnection(t *testing.T) {
	_, conn := tcpTestServer(t)
	address := conn.RemoteAddr().String()

	for name, tc := range map[string]struct {
		packet    []byte
		closeSend bool // the client stops sending, so the server sees the body end early
	}{
		"length 0":                 {packet: []byte{0, 1, 0, 0, 0, 0, 1}},
		"length 1, no function":    {packet: []byte{0, 1, 0, 0, 0, 1, 1}},
		"length above 254":         {packet: []byte{0, 1, 0, 0, 0, 255, 1}},
		"protocol identifier 1":    {packet: []byte{0, 1, 0, 1, 0, 6, 1, 3, 0, 0, 0, 1}},
		"body shorter than length": {packet: []byte{0, 1, 0, 0, 0, 6, 1, 3, 0}, closeSend: true},
	} {
		c, err := net.Dial("tcp", address)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = c.Write(tc.packet); err != nil {
			t.Fatal(err)
		}
		if tc.closeSend {
			_ = c.(*net.TCPConn).CloseWrite()
		}
		expectClosed(t, c, name)
		_ = c.Close()
	}

	readRegister(t, conn, 1)
}

// TestConnectionLimit: beyond defaultMaxConnections a new client is disconnected at once,
// and a slot is free again as soon as a client leaves.
func TestConnectionLimit(t *testing.T) {
	s, conn := tcpTestServer(t)
	address := conn.RemoteAddr().String()

	conns := make([]net.Conn, 0, defaultMaxConnections-1)
	for len(conns) < defaultMaxConnections-1 {
		c, err := net.Dial("tcp", address)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		conns = append(conns, c)
	}
	waitForClients(t, s, defaultMaxConnections)

	extra, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer extra.Close()
	expectClosed(t, extra, "client beyond the limit")

	_ = conns[0].Close()
	waitForClients(t, s, defaultMaxConnections-1)
	again, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	readRegister(t, again, 1)
}

func waitForClients(t *testing.T, s *Server, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for s.Stats().TCPClients != want {
		if time.Now().After(deadline) {
			t.Fatalf("TCPClients = %d, want %d", s.Stats().TCPClients, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
