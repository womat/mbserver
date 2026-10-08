# framereader

Package `framereader` turns a byte stream into frames separated by silence, as Modbus RTU and
similar serial protocols need: a frame ends when no byte arrived for the inter-frame delay.

```go
port, _ := serial.Open("/dev/ttyUSB0", &serial.Mode{BaudRate: 9600})
rw := framereader.NewReadWriteCloser(port, 5*time.Second, 4*time.Millisecond)

buf := make([]byte, 256)
n, err := rw.Read(buf) // one complete frame
switch {
case errors.Is(err, framereader.ErrTimeout): // nothing arrived within 5 s; read again
case err != nil: // io.EOF: closed, or the source failed
}
_, _ = rw.Write(response) // discards input left over from the previous frame first
```

- `Read` returns one frame, `ErrTimeout` when nothing arrived within the timeout, and `io.EOF`
  once the reader is closed or the source ended or failed. Bytes received before the source
  ended are still delivered.
- `Write` flushes pending input before writing, so a late echo or noise does not mix into the
  next frame.
- `Close` stops the reader goroutines and closes the source.
