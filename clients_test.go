package mbserver

import (
	"net"
	"testing"
	"time"
)

// TestClientsAndRemoteAddr: Clients lists every open TCP connection with its answered requests,
// and a function handler sees the address of the client that sent the frame.
func TestClientsAndRemoteAddr(t *testing.T) {
	s, conn := tcpTestServer(t)

	seen := make(chan net.Addr, 1)
	s.RegisterFunctionHandler(3, func(s *Server, frame Framer) ([]byte, Exception) {
		if r, ok := frame.(interface{ RemoteAddr() net.Addr }); ok {
			seen <- r.RemoteAddr()
		}
		return ReadHoldingRegisters(s, frame)
	})

	readRegister(t, conn, 1)
	if got := <-seen; got == nil || got.String() != conn.LocalAddr().String() {
		t.Errorf("handler saw remote %v, want %v", got, conn.LocalAddr())
	}

	clients := s.Clients()
	if len(clients) != 1 || clients[0].Remote.String() != conn.LocalAddr().String() || clients[0].Requests != 1 {
		t.Fatalf("Clients() = %+v, want one client %v with 1 request", clients, conn.LocalAddr())
	}
	if clients[0].Since.IsZero() || time.Since(clients[0].Since) > time.Minute {
		t.Errorf("Since = %v, want the time the connection was accepted", clients[0].Since)
	}

	// A second connection is listed after the first; closing it removes it again.
	second, err := net.Dial("tcp", conn.RemoteAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	readRegister(t, second, 1)
	<-seen
	if clients = s.Clients(); len(clients) != 2 || clients[1].Remote.String() != second.LocalAddr().String() {
		t.Fatalf("Clients() = %+v, want the second connection listed last", clients)
	}
	_ = second.Close()
	deadline := time.Now().Add(2 * time.Second)
	for len(s.Clients()) != 1 {
		if time.Now().After(deadline) {
			t.Fatalf("Clients() = %+v after closing the second connection, want one client", s.Clients())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestClientsNotCountedForUnknownUnit: like Stats, a rejected request does not count.
func TestClientsNotCountedForUnknownUnit(t *testing.T) {
	s, conn := tcpTestServer(t)
	readRegister(t, conn, 9)
	if clients := s.Clients(); len(clients) != 1 || clients[0].Requests != 0 {
		t.Fatalf("Clients() = %+v, want one client with 0 requests", clients)
	}
}
