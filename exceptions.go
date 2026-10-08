package mbserver

import "fmt"

// Exception is a Modbus exception code, named as in the Modbus Application Protocol
// Specification V1.1b3, section 7.
type Exception uint8

const (
	// Success means the request was processed; it is not sent on the wire.
	Success Exception = 0
	// IllegalFunction: the function code is not supported by the server.
	IllegalFunction Exception = 1
	// IllegalDataAddress: the requested address range is not available on the server.
	IllegalDataAddress Exception = 2
	// IllegalDataValue: a value in the request, e.g. the quantity, is not allowed.
	IllegalDataValue Exception = 3
	// ServerDeviceFailure: an unrecoverable error occurred while the server processed the request.
	ServerDeviceFailure Exception = 4
	// Acknowledge: the server accepted a long-running request and is still processing it.
	Acknowledge Exception = 5
	// ServerDeviceBusy: the server is processing a long-running command; the client should retry later.
	ServerDeviceBusy Exception = 6
	// NegativeAcknowledge: the server cannot perform the requested program function.
	NegativeAcknowledge Exception = 7
	// MemoryParityError: the server detected a parity error in its memory.
	MemoryParityError Exception = 8

	// 9 is reserved by the Modbus specification.

	// GatewayPathUnavailable: a gateway could not allocate a path to the target device.
	GatewayPathUnavailable Exception = 10
	// GatewayTargetDeviceFailedToRespond: no device answered for the requested unit ID. The
	// server sends it over Modbus TCP for unit IDs it does not serve.
	GatewayTargetDeviceFailedToRespond Exception = 11
)

func (e Exception) Error() string {
	return fmt.Sprintf("modbus exception %d: %s", e, e.String())
}

func (e Exception) String() string {
	switch e {
	case Success:
		return "Success"
	case IllegalFunction:
		return "IllegalFunction"
	case IllegalDataAddress:
		return "IllegalDataAddress"
	case IllegalDataValue:
		return "IllegalDataValue"
	case ServerDeviceFailure:
		return "ServerDeviceFailure"
	case Acknowledge:
		return "Acknowledge"
	case ServerDeviceBusy:
		return "ServerDeviceBusy"
	case NegativeAcknowledge:
		return "NegativeAcknowledge"
	case MemoryParityError:
		return "MemoryParityError"
	case GatewayPathUnavailable:
		return "GatewayPathUnavailable"
	case GatewayTargetDeviceFailedToRespond:
		return "GatewayTargetDeviceFailedToRespond"
	default:
		return "unknown"
	}
}
