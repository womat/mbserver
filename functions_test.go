package mbserver

import (
	"encoding/json"
	"log/slog"
	"testing"
)

func isEqual(a interface{}, b interface{}) bool {
	expect, _ := json.Marshal(a)
	got, _ := json.Marshal(b)

	return string(expect) == string(got)
}

// Function 1
func TestReadCoils(t *testing.T) {
	const deviceid = 247
	s := NewServer(slog.Default())
	_ = s.NewUnit(deviceid)
	// Set the coil values
	s.units[deviceid].Coils[10] = 1
	s.units[deviceid].Coils[11] = 1
	s.units[deviceid].Coils[17] = 1
	s.units[deviceid].Coils[18] = 1

	var frame TCPFrame
	frame.TransactionIdentifier = 1
	frame.ProtocolIdentifier = 0
	frame.Length = 6
	frame.UnitId = deviceid
	frame.Function = 1
	SetRegisterData(&frame, 10, 9)

	var req Request
	req.frame = &frame
	response := s.handle(req)

	exception := GetException(response)
	if exception != Success {
		t.Errorf("expected Success, got %v", exception.String())
		t.FailNow()
	}
	// 2 bytes, 0b1000011, 0b00000001
	expect := []byte{2, 131, 1}
	got := response.GetData()
	if !isEqual(expect, got) {
		t.Errorf("expected %v, got %v", expect, got)
	}
}

// Function 2
func TestReadDiscreteInputs(t *testing.T) {
	const deviceid = 240

	s := NewServer(slog.Default())
	_ = s.NewUnit(deviceid)
	// Set the discrete input values
	s.units[deviceid].DiscreteInputs[0] = 1
	s.units[deviceid].DiscreteInputs[7] = 1
	s.units[deviceid].DiscreteInputs[8] = 1
	s.units[deviceid].DiscreteInputs[9] = 1

	var frame TCPFrame
	frame.TransactionIdentifier = 1
	frame.ProtocolIdentifier = 0
	frame.Length = 6
	frame.UnitId = deviceid
	frame.Function = 2
	SetRegisterData(&frame, 0, 10)

	var req Request
	req.frame = &frame
	response := s.handle(req)

	exception := GetException(response)
	if exception != Success {
		t.Errorf("expected Success, got %v", exception.String())
		t.FailNow()
	}
	expect := []byte{2, 129, 3}
	got := response.GetData()
	if !isEqual(expect, got) {
		t.Errorf("expected %v, got %v", expect, got)
	}
}

// Function 3
func TestReadHoldingRegisters(t *testing.T) {
	const deviceid = 240
	s := NewServer(slog.Default())
	_ = s.NewUnit(deviceid)

	s.units[deviceid].HoldingRegisters[100] = 1
	s.units[deviceid].HoldingRegisters[101] = 2
	s.units[deviceid].HoldingRegisters[102] = 65535

	var frame TCPFrame
	frame.TransactionIdentifier = 1
	frame.ProtocolIdentifier = 0
	frame.Length = 6
	frame.UnitId = deviceid
	frame.Function = 3
	SetRegisterData(&frame, 100, 3)

	var req Request
	req.frame = &frame
	response := s.handle(req)
	exception := GetException(response)
	if exception != Success {
		t.Errorf("expected Success, got %v", exception.String())
		t.FailNow()
	}
	expect := []byte{6, 0, 1, 0, 2, 255, 255}
	got := response.GetData()
	if !isEqual(expect, got) {
		t.Errorf("expected %v, got %v", expect, got)
	}
}

// Function 4
func TestReadInputRegisters(t *testing.T) {
	s := NewServer(slog.Default())

	s.units[1].InputRegisters[200] = 1
	s.units[1].InputRegisters[201] = 2
	s.units[1].InputRegisters[202] = 65535

	var frame TCPFrame
	frame.TransactionIdentifier = 1
	frame.ProtocolIdentifier = 0
	frame.Length = 6
	frame.UnitId = 1
	frame.Function = 4
	SetRegisterData(&frame, 200, 3)

	var req Request
	req.frame = &frame
	response := s.handle(req)
	exception := GetException(response)
	if exception != Success {
		t.Errorf("expected Success, got %v", exception.String())
		t.FailNow()
	}
	expect := []byte{6, 0, 1, 0, 2, 255, 255}
	got := response.GetData()
	if !isEqual(expect, got) {
		t.Errorf("expected %v, got %v", expect, got)
	}
}

// Function 5
func TestWriteSingleCoil(t *testing.T) {
	s := NewServer(slog.Default())

	var frame TCPFrame
	frame.TransactionIdentifier = 1
	frame.ProtocolIdentifier = 0
	frame.Length = 12
	frame.UnitId = 1
	frame.Function = 5
	SetRegisterData(&frame, 65535, 0xFF00)

	var req Request
	req.frame = &frame
	response := s.handle(req)
	exception := GetException(response)
	if exception != Success {
		t.Errorf("expected Success, got %v", exception.String())
		t.FailNow()
	}
	expect := 1
	got := s.units[1].Coils[65535]
	if !isEqual(expect, got) {
		t.Errorf("expected %v, got %v\n", expect, got)
	}

	// 0x0000 switches the coil off again
	SetRegisterData(&frame, 65535, 0x0000)
	if exception = GetException(s.handle(req)); exception != Success || s.units[1].Coils[65535] != 0 {
		t.Errorf("0x0000: got %v and coil %d, want Success and 0", exception, s.units[1].Coils[65535])
	}

	// any other value is no coil state and leaves the coil as it is
	SetRegisterData(&frame, 65535, 0x0001)
	if exception = GetException(s.handle(req)); exception != IllegalDataValue || s.units[1].Coils[65535] != 0 {
		t.Errorf("0x0001: got %v and coil %d, want IllegalDataValue and 0", exception, s.units[1].Coils[65535])
	}
}

// Function 6
func TestWriteHoldingRegister(t *testing.T) {
	s := NewServer(slog.Default())

	var frame TCPFrame
	frame.TransactionIdentifier = 1
	frame.ProtocolIdentifier = 0
	frame.Length = 12
	frame.UnitId = 1
	frame.Function = 6
	SetRegisterData(&frame, 5, 6)

	var req Request
	req.frame = &frame
	response := s.handle(req)
	exception := GetException(response)
	if exception != Success {
		t.Errorf("expected Success, got %v", exception.String())
		t.FailNow()
	}
	expect := 6
	got := s.units[1].HoldingRegisters[5]
	if !isEqual(expect, got) {
		t.Errorf("expected %v, got %v\n", expect, got)
	}
}

// Function 15
func TestWriteMultipleCoils(t *testing.T) {
	s := NewServer(slog.Default())

	var frame TCPFrame
	frame.TransactionIdentifier = 1
	frame.ProtocolIdentifier = 0
	frame.Length = 12
	frame.UnitId = 1
	frame.Function = 15
	SetRegisterBytes(&frame, 1, 2, []byte{3})

	var req Request
	req.frame = &frame
	response := s.handle(req)
	exception := GetException(response)
	if exception != Success {
		t.Errorf("expected Success, got %v", exception.String())
		t.FailNow()
	}
	expect := []byte{1, 1}
	got := s.units[1].Coils[1:3]
	if !isEqual(expect, got) {
		t.Errorf("expected %v, got %v\n", expect, got)
	}
}

// Function 16
func TestWriteHoldingRegisters(t *testing.T) {
	const deviceid = 211

	s := NewServer(slog.Default())
	_ = s.NewUnit(deviceid)
	var frame TCPFrame
	frame.TransactionIdentifier = 1
	frame.ProtocolIdentifier = 0
	frame.Length = 12
	frame.UnitId = deviceid
	frame.Function = 16
	SetRegisterValues(&frame, 1, 2, []uint16{3, 4})

	var req Request
	req.frame = &frame
	response := s.handle(req)
	exception := GetException(response)
	if exception != Success {
		t.Errorf("expected Success, got %v", exception.String())
		t.FailNow()
	}
	expect := []uint16{3, 4}
	got := s.units[deviceid].HoldingRegisters[1:3]
	if !isEqual(expect, got) {
		t.Errorf("expected %v, got %v\n", expect, got)
	}
}

func TestBytesToUint16(t *testing.T) {
	bytes := []byte{1, 2, 3, 4}
	got := BytesToUint16(bytes)
	expect := []uint16{258, 772}
	if !isEqual(expect, got) {
		t.Errorf("expected %v, got %v\n", expect, got)
	}
}

func TestUint16ToBytes(t *testing.T) {
	values := []uint16{1, 2, 3}
	got := Uint16ToBytes(values)
	expect := []byte{0, 1, 0, 2, 0, 3}
	if !isEqual(expect, got) {
		t.Errorf("expected %v, got %v\n", expect, got)
	}
}

func TestOutOfBounds(t *testing.T) {
	s := NewServer(slog.Default())

	var frame TCPFrame
	frame.TransactionIdentifier = 1
	frame.ProtocolIdentifier = 0
	frame.Length = 6
	frame.UnitId = 1

	var req Request
	req.frame = &frame

	// bits
	SetRegisterData(&frame, 65535, 2)

	frame.Function = 1
	response := s.handle(req)
	exception := GetException(response)
	if exception != IllegalDataAddress {
		t.Errorf("expected IllegalDataAddress, got %v", exception.String())
	}

	frame.Function = 2
	response = s.handle(req)
	exception = GetException(response)
	if exception != IllegalDataAddress {
		t.Errorf("expected IllegalDataAddress, got %v", exception.String())
	}

	SetRegisterBytes(&frame, 65535, 2, []byte{3})
	frame.Function = 15
	response = s.handle(req)
	exception = GetException(response)
	if exception != IllegalDataAddress {
		t.Errorf("expected IllegalDataAddress, got %v", exception.String())
	}

	// registers
	SetRegisterData(&frame, 65535, 2)

	frame.Function = 3
	response = s.handle(req)
	exception = GetException(response)
	if exception != IllegalDataAddress {
		t.Errorf("expected IllegalDataAddress, got %v", exception.String())
	}

	frame.Function = 4
	response = s.handle(req)
	exception = GetException(response)
	if exception != IllegalDataAddress {
		t.Errorf("expected IllegalDataAddress, got %v", exception.String())
	}

	SetRegisterValues(&frame, 65535, 2, []uint16{0, 0})
	frame.Function = 16
	response = s.handle(req)
	exception = GetException(response)
	if exception != IllegalDataAddress {
		t.Errorf("expected IllegalDataAddress, got %v", exception.String())
	}
}

// writeRequest builds a request for unit 1 with address 0, quantity, and - for the write
// functions 15 and 16 - the byte count and data that match the quantity.
func writeRequest(function uint8, quantity int) *TCPFrame {
	data := []byte{0, 0, byte(quantity >> 8), byte(quantity)}
	switch function {
	case 15:
		n := (quantity + 7) / 8
		data = append(append(data, byte(n)), make([]byte, n)...)
	case 16:
		n := 2 * quantity
		data = append(append(data, byte(n)), make([]byte, n)...)
	}
	frame := &TCPFrame{TransactionIdentifier: 1, UnitId: 1, Function: function}
	frame.SetData(data)
	return frame
}

// TestQuantityLimits: every function code accepts its maximum quantity and answers 0 and
// maximum+1 with IllegalDataValue, as the specification fixes them.
func TestQuantityLimits(t *testing.T) {
	s := NewServer(quietLogger())
	for _, tc := range []struct {
		function uint8
		max      int
	}{{1, 2000}, {2, 2000}, {3, 125}, {4, 125}, {15, 1968}, {16, 123}} {
		for quantity, want := range map[int]Exception{0: IllegalDataValue, tc.max: Success, tc.max + 1: IllegalDataValue} {
			response := s.handle(Request{frame: writeRequest(tc.function, quantity)})
			if got := GetException(response); got != want {
				t.Errorf("function %d, quantity %d: got %v, want %v", tc.function, quantity, got, want)
			}
		}
	}
}

// TestWriteByteCountMismatch: FC15 and FC16 answer a byte count field or a data length that
// does not match the quantity with IllegalDataValue and write nothing.
func TestWriteByteCountMismatch(t *testing.T) {
	for name, tc := range map[string]struct {
		function uint8
		data     []byte // address 0x0001, quantity 2, byte count, values
	}{
		"FC15 byte count too large":   {15, []byte{0, 1, 0, 2, 2, 0xFF, 0xFF}},
		"FC15 more data than counted": {15, []byte{0, 1, 0, 2, 1, 0xFF, 0xFF}},
		"FC16 byte count too small":   {16, []byte{0, 1, 0, 2, 3, 0, 7, 0, 7}},
		"FC16 odd data length":        {16, []byte{0, 1, 0, 2, 4, 0, 7, 0}},
	} {
		s := NewServer(quietLogger())
		frame := &TCPFrame{TransactionIdentifier: 1, UnitId: 1, Function: tc.function}
		frame.SetData(tc.data)
		response := s.handle(Request{frame: frame})
		if got := GetException(response); got != IllegalDataValue {
			t.Errorf("%s: got %v, want IllegalDataValue", name, got)
		}
		if s.units[1].Coils[1] != 0 || s.units[1].HoldingRegisters[1] != 0 {
			t.Errorf("%s: a rejected request was written", name)
		}
	}
}
