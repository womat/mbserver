package mbserver

import "testing"

func TestNewTCPFrame(t *testing.T) {
	// transaction 0x1234, protocol 0, length 6, unit 0x15, FC4, address 0x0002, quantity 0x0003
	frame, err := NewTCPFrame([]byte{0x12, 0x34, 0x00, 0x00, 0x00, 0x06, 0x15, 0x04, 0x00, 0x02, 0x00, 0x03})
	if err != nil {
		t.Fatalf("expected %v, got %v", nil, err)
	}

	expect := TCPFrame{TransactionIdentifier: 0x1234, Length: 6, UnitId: 0x15, Function: 4, Data: []byte{0x00, 0x02, 0x00, 0x03}}
	if !isEqual(expect, *frame) {
		t.Errorf("expected %+v, got %+v", expect, *frame)
	}
}

func TestNewTCPFrameShortPacket(t *testing.T) {
	// MBAP header and unit ID without a function code
	_, err := NewTCPFrame([]byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x02, 0x01, 0x04})
	if err == nil {
		t.Fatalf("expected error not nil, got %v", err)
	}
}

func TestTCPFrameBytes(t *testing.T) {
	frame := &TCPFrame{
		TransactionIdentifier: 0x1234,
		UnitId:                uint8(1),
		Function:              uint8(4),
		Data:                  []byte{0x02, 0xff, 0xff},
	}

	// the length field is computed from the data: unit ID, function code and 3 data bytes
	got := frame.Bytes()
	expect := []byte{0x12, 0x34, 0x00, 0x00, 0x00, 0x05, 0x01, 0x04, 0x02, 0xFF, 0xFF}
	if !isEqual(expect, got) {
		t.Errorf("expected %v, got %v", expect, got)
	}
}

// TestNewTCPFrameRejects: a packet with a protocol identifier other than 0 or a length field
// that does not match the data is no Modbus TCP frame. The server reads exactly the bytes the
// length field announces, so the length check is reached only by calling NewTCPFrame directly.
func TestNewTCPFrameRejects(t *testing.T) {
	for name, packet := range map[string][]byte{
		"protocol identifier 1":  {0, 1, 0, 1, 0, 6, 1, 3, 0, 0, 0, 1},
		"length field too large": {0, 1, 0, 0, 0, 7, 1, 3, 0, 0, 0, 1},
		"length field too small": {0, 1, 0, 0, 0, 5, 1, 3, 0, 0, 0, 1},
	} {
		if _, err := NewTCPFrame(packet); err == nil {
			t.Errorf("%s: NewTCPFrame accepted % x", name, packet)
		}
	}
	if _, err := NewTCPFrame([]byte{0, 1, 0, 0, 0, 6, 1, 3, 0, 0, 0, 1}); err != nil {
		t.Errorf("valid frame rejected: %v", err)
	}
}
