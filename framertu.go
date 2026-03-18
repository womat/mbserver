package mbserver

import (
	"encoding/binary"
	"fmt"
)

// RTUFrame is the Modbus RTU frame.
type RTUFrame struct {
	Address  uint8
	Function uint8
	Data     []byte
}

// NewRTUFrame converts a packet to a Modbus RTU frame.
func NewRTUFrame(packet []byte) (*RTUFrame, error) {
	// Check the that the packet length.
	if len(packet) < 5 {
		return nil, fmt.Errorf("RTU Frame error: packet less than 5 bytes: %v", packet)
	}

	// Check the CRC.
	pLen := len(packet)
	crcExpect := binary.LittleEndian.Uint16(packet[pLen-2 : pLen])
	crcCalc := crcModbus(packet[0 : pLen-2])
	if crcCalc != crcExpect {
		return nil, fmt.Errorf("RTU frame CRC mismatch: packet=0x%04x calculated=0x%04x", crcExpect, crcCalc)
	}

	data := make([]byte, pLen-4)
	copy(data, packet[2:pLen-2])

	frame := &RTUFrame{
		Address:  packet[0],
		Function: packet[1],
		Data:     data,
	}

	return frame, nil
}

// Copy the RTUFrame.
func (frame *RTUFrame) Copy() Framer {
	c := *frame
	c.Data = make([]byte, len(frame.Data))
	copy(c.Data, frame.Data)
	return &c
}

// Bytes returns the Modbus byte stream based on the RTUFrame fields
func (frame *RTUFrame) Bytes() []byte {

	buf := make([]byte, 0, 2+len(frame.Data)+2) // 2 bytes for address and function, len(frame.Data) for data, 2 bytes for CRC
	buf = append(buf, frame.Address, frame.Function)
	buf = append(buf, frame.Data...)

	crc := crcModbus(buf)
	buf = binary.LittleEndian.AppendUint16(buf, crc)
	return buf
}

// GetDevice returns the Modbus DeviceId.
func (frame *RTUFrame) GetDevice() uint8 {
	return frame.Address
}

// SetDevice set the RTUFrame Modbus DeviceId.
func (frame *RTUFrame) SetDevice(id uint8) {
	frame.Address = id
}

// GetFunction returns the Modbus function code.
func (frame *RTUFrame) GetFunction() uint8 {
	return frame.Function
}

// GetData returns the RTUFrame Data byte field.
func (frame *RTUFrame) GetData() []byte {
	return frame.Data
}

// SetData sets the RTUFrame Data byte field and updates the frame length
// accordingly.
func (frame *RTUFrame) SetData(data []byte) {
	frame.Data = data
}

// SetException sets the Modbus exception code in the frame.
func (frame *RTUFrame) SetException(exception Exception) {
	frame.Function |= 0x80
	frame.Data = []byte{byte(exception)}
}
