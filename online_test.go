package mbserver

import (
	"context"
	"net"
	"testing"
	"time"
)

// tcpTestServer starts a server with a TCP listener on a free local port and returns a connected
// client.
func tcpTestServer(t *testing.T) (*Server, net.Conn) {
	t.Helper()
	s := NewServer(quietLogger())
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)

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
	t.Cleanup(func() { _ = conn.Close() })
	return s, conn
}

// readRegister sends FC03 for one register of unit and returns the response PDU after the
// unit ID: function code and data, or function|0x80 and the exception.
func readRegister(t *testing.T, conn net.Conn, unit byte) []byte {
	t.Helper()
	request := TCPFrame{TransactionIdentifier: 1, Device: unit, Function: 3}
	SetRegisterValues(&request, 0, 1, []uint16{})
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write(request.Bytes()); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, 32)
	n, err := conn.Read(response)
	if err != nil {
		t.Fatalf("no answer for unit %d: %v", unit, err)
	}
	return response[7:n]
}

func TestSetOnline(t *testing.T) {
	s, conn := tcpTestServer(t)
	if err := s.SetHoldingRegisters(1, 0, []uint16{42}); err != nil {
		t.Fatal(err)
	}

	if err := s.SetOnline(1, false); err != nil {
		t.Fatal(err)
	}
	if s.Online(1) {
		t.Error("Online(1) = true after SetOnline(1, false)")
	}
	if pdu := readRegister(t, conn, 1); pdu[0] != 3|0x80 || Exception(pdu[1]) != GatewayTargetDeviceFailedToRespond {
		t.Fatalf("offline unit answered % x, want exception GatewayTargetDeviceFailedToRespond", pdu)
	}

	// The registers survive and can still be written while offline.
	if err := s.UpdateHoldingRegisters(1, func(r []uint16) error { r[0]++; return nil }); err != nil {
		t.Fatalf("UpdateHoldingRegisters on an offline unit: %v", err)
	}

	if err := s.SetOnline(1, true); err != nil {
		t.Fatal(err)
	}
	if pdu := readRegister(t, conn, 1); pdu[0] != 3 || pdu[2] != 0 || pdu[3] != 43 {
		t.Fatalf("unit back online answered % x, want register value 43", pdu)
	}
}

func TestSetOnlineUnknownUnit(t *testing.T) {
	s := NewServer(quietLogger())
	if err := s.SetOnline(9, false); err == nil {
		t.Error("SetOnline for an unknown unit ID succeeded")
	}
	if s.Online(9) {
		t.Error("Online(9) = true for an unknown unit ID")
	}
}

func TestRemoveDeviceForgetsOffline(t *testing.T) {
	s := NewServer(quietLogger())
	if err := s.NewDevice(5); err != nil {
		t.Fatal(err)
	}
	_ = s.SetOnline(5, false)
	if err := s.RemoveDevice(5); err != nil {
		t.Fatal(err)
	}
	if err := s.NewDevice(5); err != nil {
		t.Fatal(err)
	}
	if !s.Online(5) {
		t.Error("a re-created unit ID is still offline")
	}
}

func TestRTUOfflineUnitIsSilent(t *testing.T) {
	f := newFakeSerial()
	s := newSerialTestServer(t, f)
	if err := s.ListenRTU(context.Background(), "/dev/ttyFAKE", SerialConfig{}); err != nil {
		t.Fatal(err)
	}
	_ = s.SetOnline(1, false)
	if response := rtuRequest(t, <-f.clients, 1, 3, 200*time.Millisecond); response != nil {
		t.Errorf("answer % x from an offline unit, want none", response)
	}
}

func TestStats(t *testing.T) {
	s, conn := tcpTestServer(t)

	readRegister(t, conn, 1)
	readRegister(t, conn, 1)
	readRegister(t, conn, 9) // unknown unit: answered with an exception, not counted

	// The counter is raised after the response is written, so the client may see the answer first.
	waitFor(t, "TCPRequests reaches 2", func() bool { return s.Stats().TCPRequests == 2 })
	st := s.Stats()
	if st.TCPRequests != 2 || st.RTURequests != 0 {
		t.Errorf("Stats() = %+v, want 2 TCP and 0 RTU requests", st)
	}
	if st.TCPClients != 1 {
		t.Errorf("TCPClients = %d, want 1", st.TCPClients)
	}

	_ = conn.Close()
	waitFor(t, "TCPClients drops to 0 after the client disconnected", func() bool { return s.Stats().TCPClients == 0 })
}

func TestStatsCountsRTU(t *testing.T) {
	f := newFakeSerial()
	s := newSerialTestServer(t, f)
	if err := s.ListenRTU(context.Background(), "/dev/ttyFAKE", SerialConfig{}); err != nil {
		t.Fatal(err)
	}
	client := <-f.clients
	if response := rtuRequest(t, client, 1, 3, time.Second); response == nil {
		t.Fatal("no answer")
	}
	waitFor(t, "RTURequests reaches 1", func() bool { return s.Stats().RTURequests == 1 })
}
