package connect

// Platform-specific terminal I/O control functions are defined in:
//
//	termios_darwin.go — Darwin (macOS) using TIOCGETA / TIOCSETA
//	termios_linux.go  — Linux using TCGETS / TCSETS
//
// These provide ioctlReadTermios, ioctlWriteTermios, isTerminal,
// makeRaw, restoreTerminal, and getSize for each platform.
