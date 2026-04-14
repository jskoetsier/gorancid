package connect

import "errors"

var (
	// ErrTimeout indicates the command did not complete within the allowed time.
	ErrTimeout = errors.New("connect: command timed out")
	// ErrAuthFailed indicates SSH authentication failed.
	ErrAuthFailed = errors.New("connect: SSH authentication failed")
	// ErrNoRoute indicates the host could not be reached.
	ErrNoRoute = errors.New("connect: host unreachable")
	// ErrNoNativeTransport indicates .cloginrc has no usable ssh or telnet method for native collection.
	ErrNoNativeTransport = errors.New("connect: native transport requires an ssh or telnet method in .cloginrc")
	// ErrNoNativeSSH is an alias for ErrNoNativeTransport (deprecated name).
	ErrNoNativeSSH = ErrNoNativeTransport
)
