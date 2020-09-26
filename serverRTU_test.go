package mbserver

import (
	"encoding/hex"
	"io"
	"os"
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
	tracelog.Printf("frame %v: WRITE: %v >> expected: %v\n", testSequenz.sequenz, hex.EncodeToString(data), hex.EncodeToString(testSequenz.frames[testSequenz.sequenz].expect))

	if isEqual(testSequenz.frames[testSequenz.sequenz].expect, data) {
		testSequenz.frames[testSequenz.sequenz].ok = true
	} else {
		errorlog.Printf("frame %v: WRITE: %v >> expected: %v\n", testSequenz.sequenz, hex.EncodeToString(data), hex.EncodeToString(testSequenz.frames[testSequenz.sequenz].expect))
	}
	testSequenz.sequenz++
	return len(data), nil
}

func (ds *dataSource) Close() error {
	return nil
}

type testFrame = struct {
	frame  []byte
	expect []byte
	ok     bool
}

type testsequenz = struct {
	sequenz int
	frames  []testFrame
}

var testSequenz testsequenz

func TestListenRTU(t *testing.T) {
	SetDebug(os.Stderr, Default)
	framereader.SetDebug(os.Stderr, framereader.Default)

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

	SetDataWithRegisterAndNumberAndValues(&rtuframe, 0xffff, 1, []uint16{})
	testSequenz.frames = append(testSequenz.frames, testFrame{frame: rtuframe.Bytes(), expect: []byte{0x01, 0x03, 02, 0x00, 0x00, 0xb8, 0x44}})

	SetDataWithRegisterAndNumberAndValues(&rtuframe, 0xffff, 2, []uint16{})
	testSequenz.frames = append(testSequenz.frames, testFrame{frame: rtuframe.Bytes(), expect: []byte{0x01, 0x83, 0x02, 0xc0, 0xf1}})

	rtuframe.Address = 2
	SetDataWithRegisterAndNumberAndValues(&rtuframe, 6000, 1, []uint16{})
	testSequenz.frames = append(testSequenz.frames, testFrame{frame: rtuframe.Bytes(), expect: []byte{0x02, 0x83, 0x04, 0xb0, 0xf3}})

	rtuframe.Address = 0
	SetDataWithRegisterAndNumberAndValues(&rtuframe, 1000, 1, []uint16{})
	testSequenz.frames = append(testSequenz.frames, testFrame{frame: rtuframe.Bytes(), expect: []byte{01, 03, 0x02, 0x11, 0x22, 0x34, 0x0d}})

	rtuframe.Address = 0
	SetDataWithRegisterAndNumberAndValues(&rtuframe, 1000, 1, []uint16{})
	testSequenz.frames = append(testSequenz.frames, testFrame{frame: nil, expect: []byte{0x03, 0x03, 0x02, 0x12, 0x34, 0xcc, 0xf3}})

	source := &dataSource{
		stream: []frame{
			{
				data:           testSequenz.frames[0].frame[:3],
				framedelay:     0,
				characterdelay: 0,
			},
			{
				data:           testSequenz.frames[0].frame[3:],
				framedelay:     3 * time.Millisecond,
				characterdelay: 2 * time.Millisecond,
			},
			{
				data:           testSequenz.frames[1].frame,
				framedelay:     100 * time.Millisecond,
				characterdelay: 0 * time.Millisecond,
			},
			{
				data:           testSequenz.frames[2].frame,
				framedelay:     100 * time.Millisecond,
				characterdelay: 0 * time.Millisecond,
			}, {
				data:           testSequenz.frames[3].frame,
				framedelay:     100 * time.Millisecond,
				characterdelay: 0 * time.Millisecond,
			}, {
				data:           testSequenz.frames[4].frame,
				framedelay:     100 * time.Millisecond,
				characterdelay: 0 * time.Millisecond,
			}, {
				data:           testSequenz.frames[5].frame,
				framedelay:     100 * time.Millisecond,
				characterdelay: 0 * time.Millisecond,
			}, {
				data:           testSequenz.frames[6].frame,
				framedelay:     100 * time.Millisecond,
				characterdelay: 3 * time.Millisecond,
			}, {
				data:           testSequenz.frames[7].frame,
				framedelay:     100 * time.Millisecond,
				characterdelay: 3 * time.Millisecond,
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

	_ = serv.ListenRTUNative(reader)

	time.Sleep(1 * time.Second)

	for n, f := range testSequenz.frames {
		if !f.ok {
			t.Errorf("frame %v: result: %v\n", n, f.ok)
		}
	}
}
