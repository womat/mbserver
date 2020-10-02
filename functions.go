package mbserver

import (
	"encoding/binary"
	"encoding/hex"
)

// ReadCoils function 1, reads coils from internal memory.
func ReadCoils(s *Server, frame Framer) ([]byte, *Exception) {
	register, numRegs, endRegister := registerAddressAndNumber(frame)
	device := frame.GetDevice()

	if endRegister > 65536 {
		infolog.Printf("ReadCoils from Device %v, Address %v, quantity %v >> Exception: IllegalDataAddress, Registeraddress: %v\n", device, register, numRegs, endRegister)
		return []byte{}, &IllegalDataAddress
	}

	debuglog.Printf("ReadCoils from Device %v, Address %v, quantity %v\n", device, register, numRegs)

	dataSize := numRegs / 8
	if (numRegs % 8) != 0 {
		dataSize++
	}
	data := make([]byte, 1+dataSize)
	data[0] = byte(dataSize)
	for i, value := range s.Devices[device].Coils[register:endRegister] {
		if value != 0 {
			shift := uint(i) % 8
			data[1+i/8] |= byte(1 << shift)
		}
	}

	tracelog.Printf("response %v\n", hex.EncodeToString(data))
	return data, &Success
}

// ReadDiscreteInputs function 2, reads discrete inputs from internal memory.
func ReadDiscreteInputs(s *Server, frame Framer) ([]byte, *Exception) {
	register, numRegs, endRegister := registerAddressAndNumber(frame)
	device := frame.GetDevice()

	if endRegister > 65536 {
		infolog.Printf("ReadDiscreteInputs from Device %v, Address %v, quantity %v >> Exception: IllegalDataAddress, Registeraddress: %v\n", device, register, numRegs, endRegister)
		return []byte{}, &IllegalDataAddress
	}

	debuglog.Printf("ReadDiscreteInputs from Device %v, Address %v, quantity %v\n", device, register, numRegs)

	dataSize := numRegs / 8
	if (numRegs % 8) != 0 {
		dataSize++
	}
	data := make([]byte, 1+dataSize)
	data[0] = byte(dataSize)
	for i, value := range s.Devices[device].DiscreteInputs[register:endRegister] {
		if value != 0 {
			shift := uint(i) % 8
			data[1+i/8] |= byte(1 << shift)
		}
	}

	tracelog.Printf("response %v\n", hex.EncodeToString(data))
	return data, &Success
}

// ReadHoldingRegisters function 3, reads holding registers from internal memory.
func ReadHoldingRegisters(s *Server, frame Framer) ([]byte, *Exception) {
	register, numRegs, endRegister := registerAddressAndNumber(frame)
	device := frame.GetDevice()

	if endRegister > 65536 {
		infolog.Printf("ReadHoldingRegisters from Device %v, Address %v, quantity %v >> Exception: IllegalDataAddress, Registeraddress: %v\n", device, register, numRegs, endRegister)
		return []byte{}, &IllegalDataAddress
	}

	debuglog.Printf("ReadHoldingRegisters from Device %v, Address %v, quantity %v\n", device, register, numRegs)
	r := append([]byte{byte(numRegs * 2)}, Uint16ToBytes(s.Devices[device].HoldingRegisters[register:endRegister])...)
	tracelog.Printf("response %v\n", hex.EncodeToString(r))
	return r, &Success
}

// ReadInputRegisters function 4, reads input registers from internal memory.
func ReadInputRegisters(s *Server, frame Framer) ([]byte, *Exception) {
	register, numRegs, endRegister := registerAddressAndNumber(frame)
	device := frame.GetDevice()

	if endRegister > 65536 {
		infolog.Printf("ReadInputRegisters from Device %v, Address %v, quantity %v >> Exception: IllegalDataAddress, Registeraddress: %v\n", device, register, numRegs, endRegister)
		return []byte{}, &IllegalDataAddress
	}

	debuglog.Printf("ReadInputRegisters from Device %v, Address %v, quantity %v\n", device, register, numRegs)

	r := append([]byte{byte(numRegs * 2)}, Uint16ToBytes(s.Devices[device].InputRegisters[register:endRegister])...)
	tracelog.Printf("Response %v\n", r)
	return r, &Success
}

// WriteSingleCoil function 5, write a coil to internal memory.
func WriteSingleCoil(s *Server, frame Framer) ([]byte, *Exception) {
	register, value := registerAddressAndValue(frame)
	device := frame.GetDevice()

	// TODO Should we use 0 for off and 65,280 (FF00 in hexadecimal) for on?
	if value != 0 {
		value = 1
	}

	debuglog.Printf("WriteSingleCoil to Device %v, Address %v, value %v\n", device, register, value)

	s.Devices[device].Coils[register] = byte(value)
	r := frame.GetData()[0:4]
	tracelog.Printf("response %v\n", hex.EncodeToString(r))
	return r, &Success
}

// WriteHoldingRegister function 6, write a holding register to internal memory.
func WriteHoldingRegister(s *Server, frame Framer) ([]byte, *Exception) {
	register, value := registerAddressAndValue(frame)
	device := frame.GetDevice()

	debuglog.Printf("WriteHoldingRegister to Device %v, Address %v, value %v\n", device, register, value)

	s.Devices[device].HoldingRegisters[register] = value
	r := frame.GetData()[0:4]
	tracelog.Printf("response %v\n", hex.EncodeToString(r))
	return r, &Success
}

// WriteMultipleCoils function 15, writes holding registers to internal memory.
func WriteMultipleCoils(s *Server, frame Framer) ([]byte, *Exception) {
	register, numRegs, endRegister := registerAddressAndNumber(frame)
	device := frame.GetDevice()

	valueBytes := frame.GetData()[5:]

	if endRegister > 65536 {
		infolog.Printf("WriteMultipleCoils from Device %v, Address %v, quantity %v >> Exception: IllegalDataAddress, Registeraddress: %v\n", device, register, numRegs, endRegister)
		return []byte{}, &IllegalDataAddress
	}

	debuglog.Printf("WriteMultipleCoils to Device %v, Address %v, values %v\n", device, register, valueBytes)

	// TODO This is not correct, bits and bytes do not always align
	//if len(valueBytes)/2 != numRegs {
	//	return []byte{}, &IllegalDataAddress
	//}

	bitCount := 0
	for i, value := range valueBytes {
		for bitPos := uint(0); bitPos < 8; bitPos++ {
			s.Devices[device].Coils[register+(i*8)+int(bitPos)] = bitAtPosition(value, bitPos)
			bitCount++
			if bitCount >= numRegs {
				break
			}
		}
		if bitCount >= numRegs {
			break
		}
	}

	r := frame.GetData()[0:4]
	tracelog.Printf("response %v\n", hex.EncodeToString(r))
	return r, &Success
}

// WriteHoldingRegisters function 16, writes holding registers to internal memory.
func WriteHoldingRegisters(s *Server, frame Framer) ([]byte, *Exception) {
	register, numRegs, endRegister := registerAddressAndNumber(frame)
	device := frame.GetDevice()
	valueBytes := frame.GetData()[5:]

	if endRegister > 65536 {
		infolog.Printf("WriteHoldingRegisters from Device %v, Address %v, quantity %v >> Exception: IllegalDataAddress, Registeraddress: %v\n", device, register, numRegs, endRegister)
		return []byte{}, &IllegalDataAddress
	}
	if len(valueBytes)/2 != numRegs {
		errorlog.Printf("WriteHoldingRegisters from Device %v, Address %v, quantity %v >> Exception: IllegalDataAddress, len(valueBytes)/2 != numRegs : (%v != %v)\n", device, register, numRegs, len(valueBytes)/2, numRegs)
		return []byte{}, &IllegalDataAddress
	}

	debuglog.Printf("WriteHoldingRegisters to Device %v, Address %v, values %v\n", device, register, valueBytes)
	// Copy data to memory
	values := BytesToUint16(valueBytes)
	if valuesUpdated := copy(s.Devices[device].HoldingRegisters[register:], values); valuesUpdated != numRegs {
		errorlog.Printf("WriteHoldingRegisters from Device %v, Address %v, quantity %v >> Exception: IllegalDataAddress, valuesUpdated != numRegs: (%v != %v\n)", device, register, numRegs, valuesUpdated, numRegs)
		return []byte{}, &IllegalDataAddress
	}

	r := frame.GetData()[0:4]
	tracelog.Printf("response %v\n", hex.EncodeToString(r))
	return r, &Success
}

// BytesToUint16 converts a big endian array of bytes to an array of unit16s
func BytesToUint16(bytes []byte) []uint16 {
	values := make([]uint16, len(bytes)/2)

	for i := range values {
		values[i] = binary.BigEndian.Uint16(bytes[i*2 : (i+1)*2])
	}
	return values
}

// Uint16ToBytes converts an array of uint16s to a big endian array of bytes
func Uint16ToBytes(values []uint16) []byte {
	bytes := make([]byte, len(values)*2)

	for i, value := range values {
		binary.BigEndian.PutUint16(bytes[i*2:(i+1)*2], value)
	}
	return bytes
}

func bitAtPosition(value uint8, pos uint) uint8 {
	return (value >> pos) & 0x01
}
