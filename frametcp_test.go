package mbserver

import "testing"

func TestNewTCPFrame(t *testing.T) {
	frame, err := NewRTUFrame([]byte{0x15, 0x04, 0x02, 0xFF, 0xFF, 0x88, 0x83})
	if !isEqual(nil, err) {
		t.Fatalf("expected %v, got %v", nil, err)
	}

	got := frame.UnitId
	expect := 0x15
	if !isEqual(expect, got) {
		t.Errorf("expected %v, got %v", expect, got)
	}

	got = frame.Function
	expect = 4
	if !isEqual(expect, got) {
		t.Errorf("expected %v, got %v", expect, got)
	}
}

func TestNewTCPFrameShortPacket(t *testing.T) {
	_, err := NewRTUFrame([]byte{0x01, 0x04, 0xFF, 0xFF})
	if err == nil {
		t.Fatalf("expected error not nil, got %v", err)
	}
}

func TestNewTCPFrameBadCRC(t *testing.T) {
	// Bad CRC: 0x81 (should be 0x80)
	_, err := NewRTUFrame([]byte{0x01, 0x04, 0x02, 0xFF, 0xFF, 0xB8, 0x81})
	if err == nil {
		t.Fatalf("expected error not nil, got %v", err)
	}
}

func TestTCPFrameBytes(t *testing.T) {
	frame := &RTUFrame{
		UnitId:   uint8(1),
		Function: uint8(4),
		Data:     []byte{0x02, 0xff, 0xff},
	}

	got := frame.Bytes()
	expect := []byte{0x01, 0x04, 0x02, 0xFF, 0xFF, 0xB8, 0x80}
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
