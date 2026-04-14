//go:build darwin

package connect

import "golang.org/x/sys/unix"

func ioctlReadTermios() uint {
	return unix.TIOCGETA
}

func ioctlWriteTermios() uint {
	return unix.TIOCSETA
}
