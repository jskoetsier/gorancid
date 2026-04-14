//go:build linux

package connect

import "golang.org/x/sys/unix"

func ioctlReadTermios() uint {
	return unix.TCGETS
}

func ioctlWriteTermios() uint {
	return unix.TCSETS
}
