package mbserver

import (
	"github.com/womat/framereader"
	"io"
	"testing"
	"time"

)

type frame struct {
	data           []byte
	frameDelay     time.Duration
	characterDelay time.Duration
}

type dataSource struct {
	stream                    []frame
	currentFrame, currentData int
}

func (ds *dataSource) Read(data []byte) (int, error) {
	if ds.currentFrame >= len(ds.stream) {
		return 0, io.EOF
	}

	f := ds.stream[ds.currentFrame]

	if ds.currentData == 0 {
		time.Sleep(f.frameDelay)
	}

	if f.characterDelay == 0 {
		n := copy(data, f.data)
		ds.currentData = 0
		ds.currentFrame++
		return n, nil
	}
	if ds.currentData > 0 {
		time.Sleep(f.characterDelay)
	}
	data[0] = f.data[ds.currentData]
	ds.currentData++
	if ds.currentData >= len(f.data) {
		ds.currentFrame++
		ds.currentData = 0
	}
	return 1, nil
}

func (ds *dataSource) Write(data []byte) (int, error) {
	testSeq.frames[testSeq.sequence].got = data
	testSeq.sequence++
	return len(data), nil
}

func (ds *dataSource) Close() error {
	return nil
}

type testFrame = struct {
	frame  []byte
	expect []byte
	got    []byte
	ok     bool
}

type testSequence = struct {
	sequence int
	frames   []testFrame
}

var testSeq testSequence

func TestListenRTU(t *testing.T) {
	testSeq = testSequence{sequence: 0, frames: []testFrame{}}
	rtuFrame := RTUFrame{Address: 1, Function: 3}

	SetDataWithRegisterAndNumberAndValues(&rtuFrame, 1000, 1, []uint16{})
	testSeq.frames = append(testSeq.frames, testFrame{frame: rtuFrame.Bytes(), expect: []byte{0x01, 0x03, 0x02, 0x11, 0x22, 0x34, 0x0d}})

	SetDataWithRegisterAndNumberAndValues(&rtuFrame, 2000, 4, []uint16{})
	testSeq.frames = append(testSeq.frames, testFrame{frame: rtuFrame.Bytes(), expect: []byte{0x01, 0x03, 0x08, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0x00, 0x27, 0x11}})

	SetDataWithRegisterAndNumberAndValues(&rtuFrame, 3000, 2, []uint16{})
	testSeq.frames = append(testSeq.frames, testFrame{frame: rtuFrame.Bytes(), expect: []byte{0x01, 0x03, 0x04, 0x00, 0x00, 0x00, 0x00, 0xfa, 0x33}})

	rtuFrame = RTUFrame{Address: 3, Function: 3}
	SetDataWithRegisterAndNumberAndValues(&rtuFrame, 1000, 1, []uint16{})
	testSeq.frames = append(testSeq.frames, testFrame{frame: rtuFrame.Bytes(), expect: []byte{0x03, 0x03, 0x02, 0x12, 0x34, 0xcc, 0xf3}})

	source := &dataSource{
		stream: []frame{
			{
				data:           testSeq.frames[0].frame,
				frameDelay:     100 * time.Millisecond,
				characterDelay: 0,
			},
			{
				data:           testSeq.frames[1].frame,
				frameDelay:     10 * time.Millisecond,
				characterDelay: 1 * time.Millisecond,
			}, {
				data:           testSeq.frames[2].frame,
				frameDelay:     10 * time.Millisecond,
				characterDelay: 2 * time.Millisecond,
			}, {
				data:           testSeq.frames[3].frame,
				frameDelay:     10 * time.Millisecond,
				characterDelay: 1 * time.Millisecond,
			},
		},
	}

	reader := framereader.NewReadWriteCloser(source, time.Second, time.Millisecond*5)
	serv := NewServer()
	_ = serv.NewDevice(3)

	serv.Devices[1].HoldingRegisters[1000] = 0x1122
	serv.Devices[1].HoldingRegisters[2000] = 0x3344
	serv.Devices[1].HoldingRegisters[2001] = 0x5566
	serv.Devices[1].HoldingRegisters[2002] = 0x7788
	serv.Devices[1].HoldingRegisters[2003] = 0x9900
	serv.Devices[3].HoldingRegisters[1000] = 0x1234

	_ = serv.ListenRTU(reader)

	time.Sleep(1000 * time.Millisecond)

	for _, f := range testSeq.frames {
		if !isEqual(f.expect, f.got) {
			t.Errorf("expected %v, got %v", testSeq.frames[testSeq.sequence].expect, testSeq.frames[testSeq.sequence].got)
		}
	}
}

func TestListenRTU1(t *testing.T) {
	testSeq = testSequence{sequence: 0, frames: []testFrame{}}
	rtuFrame := RTUFrame{Address: 1, Function: 3}

	SetDataWithRegisterAndNumberAndValues(&rtuFrame, 1000, 1, []uint16{})
	testSeq.frames = append(testSeq.frames, testFrame{frame: rtuFrame.Bytes(), expect: []byte{01, 03, 0x02, 0x11, 0x22, 0x34, 0x0d}})

	SetDataWithRegisterAndNumberAndValues(&rtuFrame, 2000, 4, []uint16{})
	testSeq.frames = append(testSeq.frames, testFrame{frame: rtuFrame.Bytes(), expect: []byte{0x01, 0x03, 0x08, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0x00, 0x27, 0x11}})

	SetDataWithRegisterAndNumberAndValues(&rtuFrame, 3000, 2, []uint16{})
	testSeq.frames = append(testSeq.frames, testFrame{frame: rtuFrame.Bytes(), expect: []byte{01, 03, 04, 00, 00, 00, 00, 0xfa, 0x33}})

	SetDataWithRegisterAndNumberAndValues(&rtuFrame, 0000, 1, []uint16{})
	testSeq.frames = append(testSeq.frames, testFrame{frame: rtuFrame.Bytes(), expect: []byte{01, 03, 02, 00, 00, 0xb8, 0x44}})

	rtuFrame = RTUFrame{Address: 3, Function: 3}
	SetDataWithRegisterAndNumberAndValues(&rtuFrame, 1000, 1, []uint16{})
	testSeq.frames = append(testSeq.frames, testFrame{frame: rtuFrame.Bytes(), expect: []byte{0x03, 0x03, 0x02, 0x12, 0x34, 0xcc, 0xf3}})

	source := &dataSource{
		stream: []frame{
			// Frame 0
			{
				data:           testSeq.frames[0].frame[:3],
				frameDelay:     100 * time.Millisecond,
				characterDelay: 2 * time.Millisecond,
			},
			{
				data:           testSeq.frames[0].frame[3:],
				frameDelay:     2 * time.Millisecond,
				characterDelay: 2 * time.Millisecond,
			},
			// Frame 1
			{
				data:           testSeq.frames[1].frame[:2],
				frameDelay:     10 * time.Millisecond,
				characterDelay: 2 * time.Millisecond,
			},
			{
				data:           testSeq.frames[1].frame[2:5],
				frameDelay:     3 * time.Millisecond,
				characterDelay: 2 * time.Millisecond,
			}, {
				data:           testSeq.frames[1].frame[5:],
				frameDelay:     2 * time.Millisecond,
				characterDelay: 1 * time.Millisecond,
			},
			// Frame 3
			{
				data:           testSeq.frames[2].frame,
				frameDelay:     10 * time.Millisecond,
				characterDelay: 1 * time.Millisecond,
			},
			// Frame 4
			{
				data:           testSeq.frames[3].frame[:3],
				frameDelay:     10 * time.Millisecond,
				characterDelay: 0,
			},
			{
				data:           testSeq.frames[3].frame[3:],
				frameDelay:     2 * time.Millisecond,
				characterDelay: 2 * time.Millisecond,
			},
			// Frame 5
			{
				data:           testSeq.frames[4].frame,
				frameDelay:     10 * time.Millisecond,
				characterDelay: 0 * time.Millisecond,
			},
		},
	}

	reader := framereader.NewReadWriteCloser(source, time.Second, time.Millisecond*5)
	serv := NewServer()
	_ = serv.NewDevice(3)

	serv.Devices[1].HoldingRegisters[1000] = 0x1122
	serv.Devices[1].HoldingRegisters[2000] = 0x3344
	serv.Devices[1].HoldingRegisters[2001] = 0x5566
	serv.Devices[1].HoldingRegisters[2002] = 0x7788
	serv.Devices[1].HoldingRegisters[2003] = 0x9900
	serv.Devices[3].HoldingRegisters[1000] = 0x1234

	_ = serv.ListenRTU(reader)

	time.Sleep(1 * time.Second)

	for _, f := range testSeq.frames {
		if !isEqual(f.expect, f.got) {
			t.Errorf("expected %v, got %v", testSeq.frames[testSeq.sequence].expect, testSeq.frames[testSeq.sequence].got)
		}
	}
}
