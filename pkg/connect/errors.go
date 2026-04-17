package connect

import "errors"

var (
	// ErrTimeout indicates the command did not complete within the allowed time.
	ErrTimeout = errors.New("connect: command timed out")
	// ErrAuthFailed indicates SSH authentication failed.
	ErrAuthFailed = errors.New("connect: SSH authentication failed")
	// ErrNoRoute indicates the host could not be reached.
	ErrNoRoute = errors.New("connect: host unreachable")
	// ErrNoNativeTransport indicates no ssh or telnet method is available in .cloginrc,
	// or the device type has no parser that provides connection parameters.
	ErrNoNativeTransport = errors.New("connect: no native transport available; check .cloginrc methods and device type parser registration")
)
