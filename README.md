# Golang Modbus Server

A Modbus server for Modbus TCP and Modbus RTU (serial), originally forked from
[tbrandon/mbserver](https://github.com/tbrandon/mbserver), with:

- several unit IDs (devices), each with its own register memory
- Modbus RTU with frame detection by the t3.5 inter-frame delay (`pkg/framereader`)
- Modbus TCP with proper MBAP framing and a limit of concurrent connections
- register access under one lock: a client never reads a partly written multi-register value
- a clean `Close`: listeners, client connections and serial ports are closed, all goroutines end
- logging via `log/slog`

The server answers these function codes:

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

Every unit ID has 65536 coils, discrete inputs, holding registers and input registers, all zero at
the start. Unit ID 1 exists from the start; add more with `NewDevice`. Requests are processed one
after the other, in the order they arrive.

## Usage

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

s := mbserver.NewServer(slog.Default())
if err := s.Start(ctx); err != nil {
	log.Fatal(err)
}
defer s.Close()

if err := s.NewDevice(200); err != nil {
	log.Fatal(err)
}

// Modbus TCP
if err := s.ListenTCP(ctx, "0.0.0.0:1502"); err != nil {
	log.Fatal(err)
}

// Modbus RTU
if err := s.ListenRTU(ctx, "/dev/ttyUSB0", mbserver.SerialConfig{
	BaudRate: 9600,
	DataBits: 8,
	StopBits: mbserver.OneStopBit,
	Parity:   mbserver.NoParity,
	Timeout:  5 * time.Second, // read timeout; the server keeps listening after it
}); err != nil {
	log.Fatal(err)
}

// Update registers while clients are reading them
_ = s.SetHoldingRegisters(200, 4124, []uint16{0x03F6, 0xFAFA})

// Several registers as one consistent update
_ = s.UpdateHoldingRegisters(1, func(r []uint16) error {
	r[4096], r[4097] = 0x0003, 0x7EEC
	return nil
})
```

Modbus usually uses port 502, which needs special permissions (e.g. `CAP_NET_BIND_SERVICE`).

## Server customization

`RegisterFunctionHandler` overrides the default behavior for a function code; `nil` disables it,
the server then answers with `IllegalFunction`. A read-only server, for example:

```go
for _, code := range []uint8{5, 6, 15, 16} {
	s.RegisterFunctionHandler(code, nil)
}
```

## Benchmarks

```sh
go test -bench=.
```
