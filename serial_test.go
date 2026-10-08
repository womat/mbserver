package mbserver

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/womat/mbserver/pkg/framereader"
)

// fakeSerial hands out net.Pipe pairs as serial ports and keeps the client ends.
type fakeSerial struct {
	mu      sync.Mutex
	fail    int // the next fail calls of open return an error
	opens   int
	clients chan net.Conn
}

func newFakeSerial() *fakeSerial { return &fakeSerial{clients: make(chan net.Conn, 4)} }

func (f *fakeSerial) open(string, SerialConfig) (io.ReadWriteCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opens++
	if f.fail > 0 {
		f.fail--
		return nil, errors.New("no such device")
	}
	client, port := net.Pipe()
	f.clients <- client
	return framereader.NewReadWriteCloser(port, time.Second, 5*time.Millisecond), nil
}

func (f *fakeSerial) openCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.opens
}

func newSerialTestServer(t *testing.T, f *fakeSerial) *Server {
	t.Helper()
	s := NewServer(quietLogger())
	s.openSerial = f.open
	s.reopenInterval = 20 * time.Millisecond
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

// rtuRequest sends a read of one holding register to unit and returns the answer, or nil
// when none arrives within wait.
func rtuRequest(t *testing.T, conn net.Conn, unit uint8, function uint8, wait time.Duration) []byte {
	t.Helper()
	request := RTUFrame{Address: unit, Function: function}
	SetRegisterValues(&request, 0, 1, []uint16{})
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	if _, err := conn.Write(request.Bytes()); err != nil {
		t.Fatalf("write request: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(wait))
	response := make([]byte, 32)
	n, err := conn.Read(response)
	if err != nil {
		return nil
	}
	return response[:n]
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestSerialPortIsReopenedAfterFailure(t *testing.T) {
	f := newFakeSerial()
	s := newSerialTestServer(t, f)
	if err := s.ListenRTU(context.Background(), "/dev/ttyFAKE", SerialConfig{BaudRate: 9600}); err != nil {
		t.Fatal(err)
	}

	first := <-f.clients
	if rtuRequest(t, first, 1, 3, time.Second) == nil {
		t.Fatal("no answer on the first port")
	}
	if st := s.SerialStatus(); len(st) != 1 || !st[0].Connected || st[0].Port != "/dev/ttyFAKE" {
		t.Fatalf("SerialStatus() = %+v, want /dev/ttyFAKE connected", st)
	}

	f.mu.Lock()
	f.fail = 2 // the adapter stays away for two attempts
	f.mu.Unlock()
	_ = first.Close() // unplug

	waitFor(t, "the failure to be reported", func() bool {
		st := s.SerialStatus()
		return !st[0].Connected && st[0].Err != nil
	})
	waitFor(t, "the port to be reopened", func() bool { return s.SerialStatus()[0].Connected })
	if n := f.openCount(); n != 4 {
		t.Errorf("open called %d times, want 4 (start, two failures, success)", n)
	}

	second := <-f.clients
	if rtuRequest(t, second, 1, 3, time.Second) == nil {
		t.Error("no answer after the port was reopened")
	}
}

func TestListenRTUReportsOpenError(t *testing.T) {
	f := newFakeSerial()
	f.fail = 1
	s := newSerialTestServer(t, f)
	if err := s.ListenRTU(context.Background(), "/dev/ttyFAKE", SerialConfig{}); err == nil {
		t.Fatal("ListenRTU succeeded although the port cannot be opened")
	}
	if st := s.SerialStatus(); len(st) != 0 {
		t.Errorf("SerialStatus() = %+v, want no port", st)
	}
}

func TestCloseStopsReopening(t *testing.T) {
	f := newFakeSerial()
	s := newSerialTestServer(t, f)
	if err := s.ListenRTU(context.Background(), "/dev/ttyFAKE", SerialConfig{}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.fail = 1000
	f.mu.Unlock()
	_ = (<-f.clients).Close()
	waitFor(t, "a reopen attempt", func() bool { return f.openCount() >= 2 })

	closed := make(chan struct{})
	go func() { s.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Close blocks while the port is being reopened")
	}
	n := f.openCount()
	time.Sleep(100 * time.Millisecond)
	if f.openCount() != n {
		t.Error("the port is still reopened after Close")
	}
}

// TestRTUUnknownUnitIsSilent: on a serial bus another device may own the unit ID.
func TestRTUUnknownUnitIsSilent(t *testing.T) {
	f := newFakeSerial()
	s := newSerialTestServer(t, f)
	if err := s.ListenRTU(context.Background(), "/dev/ttyFAKE", SerialConfig{}); err != nil {
		t.Fatal(err)
	}
	if response := rtuRequest(t, <-f.clients, 9, 3, 200*time.Millisecond); response != nil {
		t.Errorf("answer % x for unit 9, want none", response)
	}
}

func TestRTUExceptionResponse(t *testing.T) {
	f := newFakeSerial()
	s := newSerialTestServer(t, f)
	if err := s.ListenRTU(context.Background(), "/dev/ttyFAKE", SerialConfig{}); err != nil {
		t.Fatal(err)
	}
	response := rtuRequest(t, <-f.clients, 1, 0x2B, time.Second) // not supported
	if len(response) != 5 || response[0] != 1 || response[1] != 0x2B|0x80 || Exception(response[2]) != IllegalFunction {
		t.Fatalf("response % x, want unit 1, function 0xab, exception IllegalFunction", response)
	}
	if crc := binary.LittleEndian.Uint16(response[3:]); crc != crcModbus(response[:3]) {
		t.Errorf("CRC %#x, want %#x", crc, crcModbus(response[:3]))
	}
}

func TestExceptionNames(t *testing.T) {
	for code, name := range map[Exception]string{
		Success: "Success", IllegalFunction: "IllegalFunction", IllegalDataAddress: "IllegalDataAddress",
		IllegalDataValue: "IllegalDataValue", ServerDeviceFailure: "ServerDeviceFailure",
		Acknowledge: "Acknowledge", ServerDeviceBusy: "ServerDeviceBusy",
		NegativeAcknowledge: "NegativeAcknowledge", MemoryParityError: "MemoryParityError",
		GatewayPathUnavailable:             "GatewayPathUnavailable",
		GatewayTargetDeviceFailedToRespond: "GatewayTargetDeviceFailedToRespond",
		Exception(9):                       "unknown",
	} {
		if got := code.String(); got != name {
			t.Errorf("Exception(%d).String() = %q, want %q", code, got, name)
		}
	}
	if got := ServerDeviceFailure.Error(); got != "modbus exception 4: ServerDeviceFailure" {
		t.Errorf("Error() = %q", got)
	}
}

func TestRemoveDevice(t *testing.T) {
	s := NewServer(quietLogger())
	if err := s.NewDevice(5); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveDevice(5); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveDevice(5); err == nil {
		t.Error("removing a device twice succeeded")
	}
	if err := s.RemoveDevice(0); err == nil {
		t.Error("removing unit ID 0 succeeded")
	}
	if _, err := s.HoldingRegisters(5, 0, 1); err == nil {
		t.Error("a removed device still has registers")
	}
}

func TestInterframeDelay(t *testing.T) {
	for baud, want := range map[int]time.Duration{
		9600:   4010416 * time.Nanosecond, // 3.5 characters of 11 bits
		19200:  2005208 * time.Nanosecond,
		115200: 1750 * time.Microsecond, // fixed above 19200 baud
	} {
		if got := interframeDelay(baud); got != want {
			t.Errorf("interframeDelay(%d) = %v, want %v", baud, got, want)
		}
	}
}

// TestFrameWithRealByteTiming: at 9600 baud a byte takes about 1.04 ms, so the bytes of one
// request arrive about a millisecond apart. With t3.5 (about 4 ms) they must still form one
// frame. A delay computed in the wrong unit (4 µs) splits the request into single bytes.
func TestFrameWithRealByteTiming(t *testing.T) {
	client, port := net.Pipe()
	defer client.Close()
	reader := framereader.NewReadWriteCloser(port, time.Second, interframeDelay(9600))
	defer reader.Close()

	request := RTUFrame{Address: 1, Function: 3}
	SetRegisterValues(&request, 4096, 2, []uint16{})
	maxGap := make(chan time.Duration, 1)
	go func() {
		var longest time.Duration
		last := time.Now()
		for i, b := range request.Bytes() {
			if i > 0 {
				time.Sleep(time.Millisecond)
			}
			if gap := time.Since(last); i > 0 && gap > longest {
				longest = gap
			}
			last = time.Now()
			_, _ = client.Write([]byte{b})
		}
		maxGap <- longest
	}()

	buffer := make([]byte, 64)
	n, err := reader.Read(buffer)
	if err != nil {
		t.Fatal(err)
	}
	if want := len(request.Bytes()); n != want {
		// Keep reading the rest, so the writer is not blocked on the pipe.
		go func() {
			for {
				if _, err := reader.Read(make([]byte, 64)); err != nil {
					return
				}
			}
		}()
		// The test sleeps 1 ms between bytes; on a loaded machine a sleep can exceed t3.5,
		// and then the split is correct. Compare with the specified t3.5, not with
		// interframeDelay, or a wrong delay would make the test skip itself.
		const t35At9600 = 4010416 * time.Nanosecond
		if gap := <-maxGap; gap >= t35At9600 {
			t.Skipf("scheduler paused %v between two bytes (t3.5 = %v); cannot test timing here", gap, t35At9600)
		}
		t.Fatalf("got % x (%d bytes), want the whole %d-byte request", buffer[:n], n, want)
	}
	if _, err = NewRTUFrame(buffer[:n]); err != nil {
		t.Error(err)
	}
}
