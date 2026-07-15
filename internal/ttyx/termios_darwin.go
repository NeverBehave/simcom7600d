//go:build darwin

package ttyx

import "golang.org/x/sys/unix"

func applyRawTermios(fd int) error {
	t, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return err
	}
	// raw mode
	t.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP |
		unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	t.Oflag &^= unix.OPOST
	t.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	t.Cflag &^= unix.CSIZE | unix.PARENB
	t.Cflag |= unix.CS8 | unix.CREAD | unix.CLOCAL
	// Block until at least one byte arrives. VMIN=0/VTIME>0 makes an idle
	// serial port return (0, nil); bufio eventually treats repeated empty
	// reads as io.ErrNoProgress and permanently stops the modem reader.
	t.Cc[unix.VMIN] = 1
	t.Cc[unix.VTIME] = 0
	return unix.IoctlSetTermios(fd, unix.TIOCSETA, t)
}
