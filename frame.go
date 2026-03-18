package mbserver

import (
	"encoding/binary"
)

const (
	idMin = 1
	idMax = 247
)

// Framer is the interface that wraps Modbus frames.
type Framer interface {
	Bytes() []byte
	Copy() Framer
	GetData() []byte
	GetDevice() uint8
	GetFunction() uint8
	SetException(exception Exception)
	SetData(data []byte)
	SetDevice(id uint8)
}

// GetException returns the Modbus exception or Success (indicating not exception).
func GetException(frame Framer) (exception Exception) {
	function := frame.GetFunction()
	if (function & 0x80) != 0 {
		exception = Exception(frame.GetData()[0])
	}
	return exception
}

func registerAddressAndNumber(frame Framer) (register int, numRegs int, endRegister int) {
	data := frame.GetData()
	if len(data) < 4 {
		return 0, 0, 0 // oder error zurückgeben
	}
	register = int(binary.BigEndian.Uint16(data[0:2]))
	numRegs = int(binary.BigEndian.Uint16(data[2:4]))
	endRegister = register + numRegs
	return register, numRegs, endRegister
}

func registerAddressAndValue(frame Framer) (int, uint16) {
	data := frame.GetData()
	if len(data) < 4 {
		return 0, 0
	}
	register := int(binary.BigEndian.Uint16(data[0:2]))
	value := binary.BigEndian.Uint16(data[2:4])
	return register, value
}

// SetRegisterData sets the RTUFrame Data byte field to hold a register and number of registers
func SetRegisterData(frame Framer, register uint16, number uint16) {
	data := make([]byte, 4)
	binary.BigEndian.PutUint16(data[0:2], register)
	binary.BigEndian.PutUint16(data[2:4], number)
	frame.SetData(data)
}

// SetRegisterValues sets the TCPFrame Data byte field to hold a register and number of registers and values
func SetRegisterValues(frame Framer, register uint16, number uint16, values []uint16) {
	data := make([]byte, 5+len(values)*2)
	binary.BigEndian.PutUint16(data[0:2], register)
	binary.BigEndian.PutUint16(data[2:4], number)
	data[4] = uint8(len(values) * 2)
	copy(data[5:], Uint16ToBytes(values))
	frame.SetData(data)
}

// SetRegisterBytes sets the TCPFrame Data byte field to hold a register and number of registers and coil bytes
func SetRegisterBytes(frame Framer, register uint16, number uint16, b []byte) {
	data := make([]byte, 5+len(b))
	binary.BigEndian.PutUint16(data[0:2], register)
	binary.BigEndian.PutUint16(data[2:4], number)
	data[4] = byte(len(b))
	copy(data[5:], b)
	frame.SetData(data)
}
