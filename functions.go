package mbserver

import (
	"encoding/binary"
	"encoding/hex"
)

// ReadCoils function 1, reads coils from internal memory.
func ReadCoils(s *Server, frame Framer) ([]byte, Exception) {
	register, numRegs, endRegister := registerAddressAndNumber(frame)
	unitId := frame.GetUnitId()

	if numRegs < 1 || numRegs > 2000 {
		return []byte{}, IllegalDataValue
	}
	if endRegister > 65536 {
		s.log.Error("ReadCoils", "unitId", unitId, "register", register, "quantity", numRegs, "endRegister", endRegister, "exception", "IllegalDataAddress")
		return []byte{}, IllegalDataAddress
	}

	dataSize := numRegs / 8
	if (numRegs % 8) != 0 {
		dataSize++
	}
	data := make([]byte, 1+dataSize)
	data[0] = byte(dataSize)

	s.mu.RLock()
	defer s.mu.RUnlock()
	dev, ok := s.units[unitId]
	if !ok {
		return []byte{}, IllegalDataAddress
	}
	for i, value := range dev.Coils[register:endRegister] {
		if value != 0 {
			shift := uint(i) % 8
			data[1+i/8] |= byte(1 << shift)
		}
	}

	s.log.Debug("ReadCoils response", "unitId", unitId, "register", register, "quantity", numRegs, "response", hex.EncodeToString(data))
	return data, Success
}

// ReadDiscreteInputs function 2, reads discrete inputs from internal memory.
func ReadDiscreteInputs(s *Server, frame Framer) ([]byte, Exception) {
	register, numRegs, endRegister := registerAddressAndNumber(frame)
	unitId := frame.GetUnitId()

	if numRegs < 1 || numRegs > 2000 {
		return []byte{}, IllegalDataValue
	}
	if endRegister > 65536 {
		s.log.Error("ReadDiscreteInputs", "unitId", unitId, "register", register, "quantity", numRegs, "endRegister", endRegister, "exception", "IllegalDataAddress")
		return []byte{}, IllegalDataAddress
	}

	dataSize := numRegs / 8
	if (numRegs % 8) != 0 {
		dataSize++
	}
	data := make([]byte, 1+dataSize)
	data[0] = byte(dataSize)
	s.mu.RLock()
	defer s.mu.RUnlock()
	dev, ok := s.units[unitId]
	if !ok {
		return []byte{}, IllegalDataAddress
	}
	for i, value := range dev.DiscreteInputs[register:endRegister] {
		if value != 0 {
			shift := uint(i) % 8
			data[1+i/8] |= byte(1 << shift)
		}
	}

	s.log.Debug("ReadDiscreteInputs response", "unitId", unitId, "register", register, "quantity", numRegs, "response", hex.EncodeToString(data))
	return data, Success
}

// ReadHoldingRegisters function 3, reads holding registers from internal memory.
func ReadHoldingRegisters(s *Server, frame Framer) ([]byte, Exception) {
	register, numRegs, endRegister := registerAddressAndNumber(frame)
	unitId := frame.GetUnitId()

	if numRegs < 1 || numRegs > 125 {
		return []byte{}, IllegalDataValue
	}
	if endRegister > 65536 {
		s.log.Error("ReadHoldingRegisters", "unitId", unitId, "register", register, "quantity", numRegs, "endRegister", endRegister, "exception", "IllegalDataAddress")
		return []byte{}, IllegalDataAddress
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	dev, ok := s.units[unitId]
	if !ok {
		return []byte{}, IllegalDataAddress
	}
	r := append([]byte{byte(numRegs * 2)}, Uint16ToBytes(dev.HoldingRegisters[register:endRegister])...)
	s.log.Debug("ReadHoldingRegisters response", "unitId", unitId, "register", register, "quantity", numRegs, "response", hex.EncodeToString(r))
	return r, Success
}

// ReadInputRegisters function 4, reads input registers from internal memory.
func ReadInputRegisters(s *Server, frame Framer) ([]byte, Exception) {
	register, numRegs, endRegister := registerAddressAndNumber(frame)
	unitId := frame.GetUnitId()

	if numRegs < 1 || numRegs > 125 {
		return []byte{}, IllegalDataValue
	}
	if endRegister > 65536 {
		s.log.Error("ReadInputRegisters", "unitId", unitId, "register", register, "quantity", numRegs, "endRegister", endRegister, "exception", "IllegalDataAddress")
		return []byte{}, IllegalDataAddress
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	dev, ok := s.units[unitId]
	if !ok {
		return []byte{}, IllegalDataAddress
	}
	r := append([]byte{byte(numRegs * 2)}, Uint16ToBytes(dev.InputRegisters[register:endRegister])...)
	s.log.Debug("ReadInputRegisters response", "unitId", unitId, "register", register, "quantity", numRegs, "response", hex.EncodeToString(r))
	return r, Success
}

// WriteSingleCoil function 5, write a coil to internal memory.
func WriteSingleCoil(s *Server, frame Framer) ([]byte, Exception) {
	// address (2) + value (2); a shorter frame would make the echo below panic
	if len(frame.GetData()) != 4 {
		return []byte{}, IllegalDataValue
	}
	register, value := registerAddressAndValue(frame)
	unitId := frame.GetUnitId()

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.units[unitId]; !ok {
		return []byte{}, IllegalDataAddress
	}
	switch value {
	case 0x0000:
		s.units[unitId].Coils[register] = 0
	case 0xFF00:
		s.units[unitId].Coils[register] = 1
	default:
		return []byte{}, IllegalDataValue
	}

	r := frame.GetData()[0:4]
	s.log.Debug("WriteSingleCoil response", "unitId", unitId, "register", register, "value", value)
	return r, Success
}

// WriteHoldingRegister function 6, write a holding register to internal memory.
func WriteHoldingRegister(s *Server, frame Framer) ([]byte, Exception) {
	// address (2) + value (2); a shorter frame would make the echo below panic
	if len(frame.GetData()) != 4 {
		return []byte{}, IllegalDataValue
	}
	register, value := registerAddressAndValue(frame)
	unitId := frame.GetUnitId()

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.units[unitId]; !ok {
		return []byte{}, IllegalDataAddress
	}
	s.units[unitId].HoldingRegisters[register] = value
	r := frame.GetData()[0:4]
	s.log.Debug("WriteHoldingRegister response", "unitId", unitId, "register", register, "value", value, "response", hex.EncodeToString(r))
	return r, Success
}

// WriteMultipleCoils function 15, writes holding registers to internal memory.
func WriteMultipleCoils(s *Server, frame Framer) ([]byte, Exception) {
	register, numRegs, endRegister := registerAddressAndNumber(frame)
	unitId := frame.GetUnitId()

	if numRegs < 1 || numRegs > 1968 {
		return []byte{}, IllegalDataValue
	}
	if endRegister > 65536 {
		s.log.Error("WriteMultipleCoils", "unitId", unitId, "register", register, "quantity", numRegs, "endRegister", endRegister, "exception", "IllegalDataAddress")
		return []byte{}, IllegalDataAddress
	}

	data := frame.GetData()
	if len(data) < 5 {
		return []byte{}, IllegalDataValue
	}
	valueBytes := data[5:]

	expectedBytes := (numRegs + 7) / 8
	if len(valueBytes) < expectedBytes {
		return []byte{}, IllegalDataValue
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.units[unitId]; !ok {
		return []byte{}, IllegalDataAddress
	}

	bitCount := 0
	for i, value := range valueBytes {
		for bitPos := uint(0); bitPos < 8; bitPos++ {
			s.units[unitId].Coils[register+(i*8)+int(bitPos)] = bitAtPosition(value, bitPos)
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
	s.log.Debug("WriteMultipleCoils response", "unitId", unitId, "register", register, "quantity", numRegs, "response", hex.EncodeToString(r))
	return r, Success
}

// WriteHoldingRegisters function 16, writes holding registers to internal memory.
func WriteHoldingRegisters(s *Server, frame Framer) ([]byte, Exception) {
	register, numRegs, endRegister := registerAddressAndNumber(frame)
	unitId := frame.GetUnitId()

	if numRegs < 1 || numRegs > 123 {
		return []byte{}, IllegalDataValue
	}
	if endRegister > 65536 {
		s.log.Error("WriteHoldingRegisters", "unitId", unitId, "register", register, "quantity", numRegs, "endRegister", endRegister, "exception", "IllegalDataAddress")
		return []byte{}, IllegalDataAddress
	}

	data := frame.GetData()
	if len(data) < 5 {
		return []byte{}, IllegalDataValue
	}
	valueBytes := data[5:]

	if len(valueBytes)/2 != numRegs {
		s.log.Error("WriteHoldingRegisters", "unitId", unitId, "register", register, "quantity", numRegs, "valueBytesLength", len(valueBytes), "exception", "IllegalDataAddress")
		return []byte{}, IllegalDataAddress
	}

	// Copy data to memory
	values := BytesToUint16(valueBytes)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.units[unitId]; !ok {
		return []byte{}, IllegalDataAddress
	}
	if valuesUpdated := copy(s.units[unitId].HoldingRegisters[register:], values); valuesUpdated != numRegs {
		s.log.Error("WriteHoldingRegisters", "unitId", unitId, "register", register, "quantity", numRegs, "valuesUpdated", valuesUpdated, "exception", "IllegalDataAddress")
		return []byte{}, IllegalDataAddress
	}

	r := frame.GetData()[0:4]
	s.log.Debug("WriteHoldingRegisters response", "unitId", unitId, "register", register, "quantity", numRegs, "response", hex.EncodeToString(r))
	return r, Success
}

// BytesToUint16 converts a big endian array of bytes to an array of unit16s
func BytesToUint16(buf []byte) []uint16 {
	values := make([]uint16, len(buf)/2)

	for i := range values {
		values[i] = binary.BigEndian.Uint16(buf[i*2 : (i+1)*2])
	}
	return values
}

// Uint16ToBytes converts an array of uint16s to a big endian array of bytes
func Uint16ToBytes(values []uint16) []byte {
	buf := make([]byte, len(values)*2)

	for i, value := range values {
		binary.BigEndian.PutUint16(buf[i*2:(i+1)*2], value)
	}
	return buf
}

func bitAtPosition(value uint8, pos uint) uint8 {
	return (value >> pos) & 0x01
}
