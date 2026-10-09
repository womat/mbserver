# mbserver

**A Modbus TCP and RTU server for Go: several unit IDs, consistent register updates, and a serial
line that survives an unplugged adapter.**

[![CI](https://github.com/womat/mbserver/actions/workflows/ci.yml/badge.svg)](https://github.com/womat/mbserver/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/womat/mbserver.svg)](https://pkg.go.dev/github.com/womat/mbserver)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue)](LICENSE)

mbserver lets a Go program answer Modbus requests from PLCs, inverters, energy managers or SCADA
clients — for example to emulate a meter or to expose values that live elsewhere. It keeps a
register memory per unit ID that the program updates while clients read it.

It started as a fork of [tbrandon/mbserver](https://github.com/tbrandon/mbserver) and is used in
production by [smartmeter](https://github.com/womat/smartmeter), a Fronius Smart Meter emulator.

## Features

- **Modbus TCP** with proper MBAP framing, several requests per connection, at most 100 clients
- **Modbus RTU** on a serial port: a request ends after the t3.5 silent interval (or a configured
  delay); the port is **reopened automatically** after a failure, e.g. an unplugged USB adapter
- **Several unit IDs**, each with 65536 coils, discrete inputs, holding and input registers
- **Consistent updates**: `UpdateHoldingRegisters` changes several registers under one lock, so a
  client never reads a 32-bit value with one new and one old word
- **Offline unit IDs**: `SetOnline(id, false)` makes a unit ID go silent while it keeps its
  registers, e.g. for a gateway whose source is lost
- **Activity counters**: `Stats` reports the answered requests per transport and the connected
  TCP clients; `Clients` lists each open TCP connection with its address and requests, and a
  handler sees the address of the client that sent a frame
- **Clean shutdown**: `Close` stops listeners, disconnects TCP clients and closes serial ports
- **Customizable** function handlers, e.g. a read-only server, or a gateway that forwards
  requests to real devices (see [Gateway](#gateway))
- Logging via `log/slog`; no cgo, runs on Linux, macOS and Windows (serial via
  [go.bug.st/serial](https://github.com/bugst/go-serial))

## Install

```sh
go get github.com/womat/mbserver@latest
```

Requires Go 1.27 or later.

## Usage

```go
package main

import (
	"context"
	"log"
	"log/slog"
	"os/signal"
	"syscall"

	"github.com/womat/mbserver"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	s := mbserver.NewServer(slog.Default()) // unit ID 1 exists from the start
	if err := s.Start(ctx); err != nil {
		log.Fatal(err)
	}
	defer s.Close()

	if err := s.NewUnit(200); err != nil {
		log.Fatal(err)
	}

	// Modbus TCP; port 502 needs root or CAP_NET_BIND_SERVICE
	if err := s.ListenTCP(ctx, "0.0.0.0:1502"); err != nil {
		log.Fatal(err)
	}

	// Modbus RTU
	if err := s.ListenRTU(ctx, "/dev/ttyUSB0", mbserver.SerialConfig{
		BaudRate: 9600,
		DataBits: 8,
		StopBits: mbserver.OneStopBit,
		Parity:   mbserver.NoParity,
	}); err != nil {
		log.Fatal(err)
	}

	// Publish a value while clients are reading
	_ = s.SetHoldingRegisters(200, 4124, []uint16{0x03F6, 0xFAFA})

	<-ctx.Done()
}
```

### Registers

| Method                                   | Description                                                        |
|------------------------------------------|--------------------------------------------------------------------|
| `NewUnit(id)` / `RemoveUnit(id)`         | add or remove a unit ID (1–247)                                    |
| `SetHoldingRegisters(id, start, values)` | write consecutive holding registers under one lock                 |
| `HoldingRegisters(id, start, quantity)`  | copy of holding registers                                          |
| `UpdateHoldingRegisters(id, func)`       | change any holding registers of a unit ID as one consistent update |
| `SetOnline(id, online)` / `Online(id)`   | take a unit ID off the bus and back, keeping its registers         |

```go
// Several registers as one update: a client reads either all old or all new values.
err := s.UpdateHoldingRegisters(1, func(r []uint16) error {
	r[4096], r[4097] = 0x0003, 0x7EEC // 229100 as uint32, high word first
	r[4134] = 500
	return nil
})
```

An offline unit ID is answered like an unknown one (see [Behavior](#behavior)); its registers can
still be written, so it comes back with current values and no moment of empty registers:

```go
_ = s.SetOnline(1, false) // the source is lost: let the client see a failed device
// ... later, with fresh values written:
_ = s.SetOnline(1, true)
```

### Activity

```go
st := s.Stats() // st.TCPRequests, st.RTURequests, st.TCPClients
```

`TCPRequests` and `RTURequests` count the requests answered since `NewServer`, exceptions included;
requests for unknown or offline unit IDs and broadcasts are not counted. Poll them and take the
difference to show activity.

```go
for _, c := range s.Clients() { // the open TCP connections, oldest first
	fmt.Println(c.Remote, c.Since, c.Requests)
}
```

A function handler sees which client sent a frame; frames from a serial line carry no address:

```go
if r, ok := frame.(interface{ RemoteAddr() net.Addr }); ok {
	slog.Debug("request", "remote", r.RemoteAddr())
}
```

### Function codes

| Code | Function                         |
|------|----------------------------------|
| 1    | Read Coils                       |
| 2    | Read Discrete Inputs             |
| 3    | Read Holding Registers           |
| 4    | Read Input Registers             |
| 5    | Write Single Coil                |
| 6    | Write Single Holding Register    |
| 15   | Write Multiple Coils             |
| 16   | Write Multiple Holding Registers |

`RegisterFunctionHandler` replaces the handler of a function code; `nil` disables it, the server then
answers with `IllegalFunction`. A read-only server:

```go
for _, code := range []uint8{5, 6, 15, 16} {
	s.RegisterFunctionHandler(code, nil)
}
```

### Gateway

A handler does not have to use the register memory: it can forward the request to a real device
and return its answer. The unit ID of the request selects the device; only unit IDs created with
`NewUnit` are passed to the handlers. Disable broadcasts, so a write to unit ID 0 cannot reach
every device at once.

```go
s := mbserver.NewServer(slog.Default())
s.SetBroadcast(false)
_ = s.RemoveUnit(1) // exists from the start; keep only the forwarded unit IDs
_ = s.NewUnit(11)

s.RegisterFunctionHandler(3, func(_ *mbserver.Server, frame mbserver.Framer) ([]byte, mbserver.Exception) {
	data := frame.GetData() // address (2 bytes) + quantity (2 bytes)
	if len(data) != 4 {
		return nil, mbserver.IllegalDataValue
	}
	values, err := readFromDevice(frame.GetUnitId(), data) // your client, e.g. Modbus RTU
	if err != nil {
		return nil, mbserver.GatewayTargetDeviceFailedToRespond
	}
	return append([]byte{byte(len(values))}, values...), mbserver.Success
})
```

Requests are processed one after the other (see [Behavior](#behavior)), so a slow device delays
the requests behind it.

### Serial line state

```go
for _, st := range s.SerialStatus() {
	fmt.Println(st.Port, st.Connected, st.Err, st.Since)
}
```

After a read error the server logs it, reports the port as not connected and tries to reopen it
every 5 seconds until it succeeds or `Close` is called. `ListenRTU` itself fails if the port cannot
be opened at all, so a wrong device name shows up at start.

## Behavior

- **Requests are processed one after the other**, in the order they arrive, across all
  connections and serial ports.
- **Unknown and offline unit IDs**: over TCP the server answers with exception 11
  (`GatewayTargetDeviceFailedToRespond`), so the client does not run into its timeout. On a serial
  bus it stays silent, because another device may own that unit ID.
- **Broadcasts** (unit ID 0) are applied to every unit ID and not answered, as specified.
  `SetBroadcast(false)` drops them instead: silence over RTU, exception 11 over TCP.
- **Malformed requests** are answered with `IllegalDataValue`, e.g. FC5 or FC6 without exactly
  address and value. A function handler that panics is answered with `ServerDeviceFailure`;
  the server logs the panic and keeps serving.
- **RTU framing**: a request ends after the silent interval t3.5 — about 4 ms at 9600 baud, 1.75 ms
  above 19200 baud. USB-RS485 adapters deliver bytes in packets with gaps of several milliseconds;
  if requests arrive split (CRC errors in the log), set `SerialConfig.InterFrameDelay`, e.g. to
  20–40 ms.
- **Read timeout** (`SerialConfig.Timeout`, 5 s by default): only bounds a single read; a client
  that is silent for longer is still answered afterwards.
- **`Close`** disconnects TCP clients, so after a restart nobody keeps being served by the old
  instance. It may be called more than once.

## Exceptions

The exception codes follow the Modbus Application Protocol Specification V1.1b3:
`IllegalFunction` (1), `IllegalDataAddress` (2), `IllegalDataValue` (3), `ServerDeviceFailure` (4),
`Acknowledge` (5), `ServerDeviceBusy` (6), `NegativeAcknowledge` (7), `MemoryParityError` (8),
`GatewayPathUnavailable` (10), `GatewayTargetDeviceFailedToRespond` (11).

## Upgrading from v0.2.x

v0.3.0 names the Modbus address after the specification, the unit identifier, instead of
"device":

| v0.2.x                                     | v0.3.0                                     |
|--------------------------------------------|--------------------------------------------|
| `NewDevice(id)` / `RemoveDevice(id)`       | `NewUnit(id)` / `RemoveUnit(id)`           |
| `type Device`                              | `type Unit`                                |
| `Framer.GetDevice()` / `SetDevice(id)`     | `Framer.GetUnitId()` / `SetUnitId(id)`     |
| `TCPFrame.Device`, `RTUFrame.Address`      | `TCPFrame.UnitId`, `RTUFrame.UnitId`       |
| log key `device`                           | log key `unitId`                           |

New: `SetBroadcast`. Fixed: FC5 and FC6 with too few data bytes panicked and stopped the process;
they are now answered with `IllegalDataValue`, and a panicking handler no longer stops the server.

## Upgrading from v0.0.x

v0.1.0 changes the API:

| v0.0.x                                  | v0.1.0                                                       |
|-----------------------------------------|--------------------------------------------------------------|
| `NewServer()`                           | `NewServer(logger)`, then `Start(ctx)`                       |
| `ListenTCP(address)`                    | `ListenTCP(ctx, address)`                                    |
| `ListenRTU(port io.ReadWriteCloser)`    | `ListenRTU(ctx, name, SerialConfig)` — the server opens the port |
| `s.Devices[id].HoldingRegisters[i] = v` | `SetHoldingRegisters` / `UpdateHoldingRegisters`             |
| `SlaveDeviceFailure`, `SlaveDeviceBusy`, `AcknowledgeSlave` | `ServerDeviceFailure`, `ServerDeviceBusy`, `Acknowledge` |
| `SetDebug(...)`                         | pass a `*slog.Logger` to `NewServer`                         |

## Development

```sh
go vet ./...
go test -race ./...
go test -bench=.      # TCP read/write benchmarks
```

`TestSerialHW` runs against a real adapter on `/dev/ttyUSB0` and is skipped without one; all
other tests use in-memory pipes and local TCP ports.

## License

MIT, see [LICENSE](LICENSE). Based on [tbrandon/mbserver](https://github.com/tbrandon/mbserver)
(MIT, © 2017 Tyler Brandon). The CRC table is derived from
[libcrc](https://github.com/lammertb/libcrc) (MIT, © 1999-2016 Lammert Bies).
