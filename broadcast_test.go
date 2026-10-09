package mbserver

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// tcpPDU sends function and data to unit and returns the response PDU after the unit ID, or
// nil when no answer arrives within wait.
func tcpPDU(t *testing.T, conn net.Conn, unit, function byte, data []byte, wait time.Duration) []byte {
	t.Helper()
	request := TCPFrame{TransactionIdentifier: 1, UnitId: unit, Function: function}
	request.SetData(data)
	_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
	if _, err := conn.Write(request.Bytes()); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(wait))
	response := make([]byte, 32)
	n, err := conn.Read(response)
	if err != nil {
		return nil
	}
	return response[7:n]
}

// writeRegisterData is the FC06 data for register and value.
func writeRegisterData(register, value uint16) []byte {
	data := make([]byte, 4)
	binary.BigEndian.PutUint16(data[0:2], register)
	binary.BigEndian.PutUint16(data[2:4], value)
	return data
}

func TestBroadcastAppliedByDefault(t *testing.T) {
	s, conn := tcpTestServer(t)
	if err := s.NewUnit(2); err != nil {
		t.Fatal(err)
	}

	if pdu := tcpPDU(t, conn, 0, 6, writeRegisterData(0, 7), 200*time.Millisecond); pdu != nil {
		t.Fatalf("broadcast answered % x, want no answer", pdu)
	}
	for _, unit := range []byte{1, 2} {
		if pdu := readRegister(t, conn, unit); pdu[0] != 3 || pdu[3] != 7 {
			t.Errorf("unit %d answered % x after the broadcast, want register value 7", unit, pdu)
		}
	}
}

func TestBroadcastDisabledTCP(t *testing.T) {
	s, conn := tcpTestServer(t)
	s.SetBroadcast(false)

	pdu := tcpPDU(t, conn, 0, 6, writeRegisterData(0, 7), time.Second)
	if pdu == nil || pdu[0] != 6|0x80 || Exception(pdu[1]) != GatewayTargetDeviceFailedToRespond {
		t.Fatalf("dropped broadcast answered % x, want exception GatewayTargetDeviceFailedToRespond", pdu)
	}
	if pdu := readRegister(t, conn, 1); pdu[0] != 3 || pdu[3] != 0 {
		t.Errorf("unit 1 answered % x, want register value 0: the broadcast must not be applied", pdu)
	}

	s.SetBroadcast(true)
	if pdu := tcpPDU(t, conn, 0, 6, writeRegisterData(0, 7), 200*time.Millisecond); pdu != nil {
		t.Fatalf("broadcast answered % x after SetBroadcast(true), want no answer", pdu)
	}
	if pdu := readRegister(t, conn, 1); pdu[3] != 7 {
		t.Errorf("unit 1 answered % x, want register value 7 after SetBroadcast(true)", pdu)
	}
}

func TestBroadcastDisabledRTUIsSilent(t *testing.T) {
	f := newFakeSerial()
	s := newSerialTestServer(t, f)
	s.SetBroadcast(false)
	if err := s.ListenRTU(context.Background(), "/dev/ttyFAKE", SerialConfig{}); err != nil {
		t.Fatal(err)
	}
	client := <-f.clients

	request := RTUFrame{UnitId: 0, Function: 6}
	request.SetData(writeRegisterData(0, 7))
	_ = client.SetDeadline(time.Now().Add(time.Second))
	if _, err := client.Write(request.Bytes()); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if n, err := client.Read(make([]byte, 32)); err == nil {
		t.Fatalf("dropped broadcast answered with %d bytes, want silence", n)
	}

	values, err := s.HoldingRegisters(1, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if values[0] != 0 {
		t.Errorf("register 0 = %d, want 0: the broadcast must not be applied", values[0])
	}
}

// TestShortWriteSingle: FC05 and FC06 with less than address and value used to panic and take
// the process down. A frame without any data byte is already rejected by NewTCPFrame, which closes
// the connection, so the cases start at one byte.
func TestShortWriteSingle(t *testing.T) {
	_, conn := tcpTestServer(t)

	for _, function := range []byte{5, 6} {
		for _, data := range [][]byte{{0x00}, {0x00, 0x01}, {0x00, 0x01, 0xFF}, {0x00, 0x01, 0xFF, 0x00, 0x00}} {
			pdu := tcpPDU(t, conn, 1, function, data, time.Second)
			if pdu == nil || pdu[0] != function|0x80 || Exception(pdu[1]) != IllegalDataValue {
				t.Errorf("FC%d with %d data bytes answered % x, want exception IllegalDataValue", function, len(data), pdu)
			}
		}
	}

	if pdu := readRegister(t, conn, 1); pdu[0] != 3 {
		t.Fatalf("server answered % x after the short writes, want a normal read", pdu)
	}
}

func TestHandlerPanicIsRecovered(t *testing.T) {
	s, conn := tcpTestServer(t)
	s.RegisterFunctionHandler(65, func(*Server, Framer) ([]byte, Exception) {
		panic("broken handler")
	})

	pdu := tcpPDU(t, conn, 1, 65, []byte{0x00}, time.Second)
	if pdu == nil || pdu[0] != 65|0x80 || Exception(pdu[1]) != ServerDeviceFailure {
		t.Fatalf("panicking handler answered % x, want exception ServerDeviceFailure", pdu)
	}
	if pdu := readRegister(t, conn, 1); pdu[0] != 3 {
		t.Fatalf("server answered % x after the panic, want a normal read", pdu)
	}
}
