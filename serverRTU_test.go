package mbserver

import (
	"io"
	"testing"
	"time"

	"github.com/womat/framereader"
)

type frame struct {
	data           []byte
	framedelay     time.Duration
	characterdelay time.Duration
}

type dataSource struct {
	stream                    []frame
	currentframe, currentdata int
}

func (ds *dataSource) Read(data []byte) (int, error) {
	if ds.currentframe >= len(ds.stream) {
		return 0, io.EOF
	}

	f := ds.stream[ds.currentframe]

	if ds.currentdata == 0 {
		time.Sleep(f.framedelay)
	}

	if f.characterdelay == 0 {
		n := copy(data, f.data)
		ds.currentdata = 0
		ds.currentframe++
		return n, nil
	} else {
		if ds.currentdata > 0 {
			time.Sleep(f.characterdelay)
		}
		data[0] = f.data[ds.currentdata]
		ds.currentdata++
		if ds.currentdata >= len(f.data) {
			ds.currentframe++
			ds.currentdata = 0
		}
		return 1, nil
	}
}

func (ds *dataSource) Write(data []byte) (int, error) {
	testSequenz.frames[testSequenz.sequenz].got = data[:len(data)]
	testSequenz.sequenz++
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

type testsequenz = struct {
	sequenz int
	frames  []testFrame
}

var testSequenz testsequenz

func TestListenRTU(t *testing.T) {
	testSequenz = testsequenz{sequenz: 0, frames: []testFrame{}}
	rtuframe := RTUFrame{Address: 1, Function: 3}

	SetDataWithRegisterAndNumberAndValues(&rtuframe, 1000, 1, []uint16{})
	testSequenz.frames = append(testSequenz.frames, testFrame{frame: rtuframe.Bytes(), expect: []byte{0x01, 0x03, 0x02, 0x11, 0x22, 0x34, 0x0d}})

	SetDataWithRegisterAndNumberAndValues(&rtuframe, 2000, 4, []uint16{})
	testSequenz.frames = append(testSequenz.frames, testFrame{frame: rtuframe.Bytes(), expect: []byte{0x01, 0x03, 0x08, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0x00, 0x27, 0x11}})

	SetDataWithRegisterAndNumberAndValues(&rtuframe, 3000, 2, []uint16{})
	testSequenz.frames = append(testSequenz.frames, testFrame{frame: rtuframe.Bytes(), expect: []byte{0x01, 0x03, 0x04, 0x00, 0x00, 0x00, 0x00, 0xfa, 0x33}})

	rtuframe = RTUFrame{Address: 3, Function: 3}
	SetDataWithRegisterAndNumberAndValues(&rtuframe, 1000, 1, []uint16{})
	testSequenz.frames = append(testSequenz.frames, testFrame{frame: rtuframe.Bytes(), expect: []byte{0x03, 0x03, 0x02, 0x12, 0x34, 0xcc, 0xf3}})

	source := &dataSource{
		stream: []frame{
			{
				data:           testSequenz.frames[0].frame,
				framedelay:     100 * time.Millisecond,
				characterdelay: 0,
			},
			{
				data:           testSequenz.frames[1].frame,
				framedelay:     10 * time.Millisecond,
				characterdelay: 1 * time.Millisecond,
			}, {
				data:           testSequenz.frames[2].frame,
				framedelay:     10 * time.Millisecond,
				characterdelay: 2 * time.Millisecond,
			}, {
				data:           testSequenz.frames[3].frame,
				framedelay:     10 * time.Millisecond,
				characterdelay: 1 * time.Millisecond,
			},
		},
	}

	reader := framereader.NewReadWriteCloser(source, time.Second, time.Millisecond*5)
	serv := NewServer()
	serv.NewDevice(3)

	serv.Devices[1].HoldingRegisters[1000] = 0x1122
	serv.Devices[1].HoldingRegisters[2000] = 0x3344
	serv.Devices[1].HoldingRegisters[2001] = 0x5566
	serv.Devices[1].HoldingRegisters[2002] = 0x7788
	serv.Devices[1].HoldingRegisters[2003] = 0x9900
	serv.Devices[3].HoldingRegisters[1000] = 0x1234

	_ = serv.ListenRTU(reader)

	time.Sleep(1000 * time.Millisecond)

	for _, f := range testSequenz.frames {
		if !isEqual(f.expect, f.got) {
			t.Errorf("expected %v, got %v", testSequenz.frames[testSequenz.sequenz].expect, testSequenz.frames[testSequenz.sequenz].got)
		}
	}
}

func TestListenRTU1(t *testing.T) {
	testSequenz = testsequenz{sequenz: 0, frames: []testFrame{}}
	rtuframe := RTUFrame{Address: 1, Function: 3}

	SetDataWithRegisterAndNumberAndValues(&rtuframe, 1000, 1, []uint16{})
	testSequenz.frames = append(testSequenz.frames, testFrame{frame: rtuframe.Bytes(), expect: []byte{01, 03, 0x02, 0x11, 0x22, 0x34, 0x0d}})

	SetDataWithRegisterAndNumberAndValues(&rtuframe, 2000, 4, []uint16{})
	testSequenz.frames = append(testSequenz.frames, testFrame{frame: rtuframe.Bytes(), expect: []byte{0x01, 0x03, 0x08, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0x00, 0x27, 0x11}})

	SetDataWithRegisterAndNumberAndValues(&rtuframe, 3000, 2, []uint16{})
	testSequenz.frames = append(testSequenz.frames, testFrame{frame: rtuframe.Bytes(), expect: []byte{01, 03, 04, 00, 00, 00, 00, 0xfa, 0x33}})

	SetDataWithRegisterAndNumberAndValues(&rtuframe, 0000, 1, []uint16{})
	testSequenz.frames = append(testSequenz.frames, testFrame{frame: rtuframe.Bytes(), expect: []byte{01, 03, 02, 00, 00, 0xb8, 0x44}})

	rtuframe = RTUFrame{Address: 3, Function: 3}
	SetDataWithRegisterAndNumberAndValues(&rtuframe, 1000, 1, []uint16{})
	testSequenz.frames = append(testSequenz.frames, testFrame{frame: rtuframe.Bytes(), expect: []byte{0x03, 0x03, 0x02, 0x12, 0x34, 0xcc, 0xf3}})

	source := &dataSource{
		stream: []frame{
			// Frame 0
			{
				data:           testSequenz.frames[0].frame[:3],
				framedelay:     100 * time.Millisecond,
				characterdelay: 2 * time.Millisecond,
			},
			{
				data:           testSequenz.frames[0].frame[3:],
				framedelay:     2 * time.Millisecond,
				characterdelay: 2 * time.Millisecond,
			},
			// Frame 1
			{
				data:           testSequenz.frames[1].frame[:2],
				framedelay:     10 * time.Millisecond,
				characterdelay: 2 * time.Millisecond,
			},
			{
				data:           testSequenz.frames[1].frame[2:5],
				framedelay:     3 * time.Millisecond,
				characterdelay: 2 * time.Millisecond,
			}, {
				data:           testSequenz.frames[1].frame[5:],
				framedelay:     2 * time.Millisecond,
				characterdelay: 1 * time.Millisecond,
			},
			// Frame 3
			{
				data:           testSequenz.frames[2].frame,
				framedelay:     10 * time.Millisecond,
				characterdelay: 1 * time.Millisecond,
			},
			//Frame 4
			{
				data:           testSequenz.frames[3].frame[:3],
				framedelay:     10 * time.Millisecond,
				characterdelay: 0,
			},
			{
				data:           testSequenz.frames[3].frame[3:],
				framedelay:     2 * time.Millisecond,
				characterdelay: 2 * time.Millisecond,
			},
			//Frame 5
			{
				data:           testSequenz.frames[4].frame,
				framedelay:     10 * time.Millisecond,
				characterdelay: 0 * time.Millisecond,
			},
		},
	}

	reader := framereader.NewReadWriteCloser(source, time.Second, time.Millisecond*5)
	serv := NewServer()
	serv.NewDevice(3)

	serv.Devices[1].HoldingRegisters[1000] = 0x1122
	serv.Devices[1].HoldingRegisters[2000] = 0x3344
	serv.Devices[1].HoldingRegisters[2001] = 0x5566
	serv.Devices[1].HoldingRegisters[2002] = 0x7788
	serv.Devices[1].HoldingRegisters[2003] = 0x9900
	serv.Devices[3].HoldingRegisters[1000] = 0x1234

	_ = serv.ListenRTU(reader)

	time.Sleep(1 * time.Second)

	for _, f := range testSequenz.frames {
		if !isEqual(f.expect, f.got) {
			t.Errorf("expected %v, got %v", testSequenz.frames[testSequenz.sequenz].expect, testSequenz.frames[testSequenz.sequenz].got)
		}
	}
}
