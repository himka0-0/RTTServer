package tcp

import (
	"fmt"
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

func TcpInfoRTT(c net.Conn) (uint32, uint32, error) {
	sc, ok := c.(syscall.Conn)
	if !ok {
		return 0, 0, fmt.Errorf("net.Conn does not implement syscall.Conn")
	}

	raw, err := sc.SyscallConn()
	if err != nil {
		return 0, 0, err
	}

	var info *unix.TCPInfo
	var serr error

	if cerr := raw.Control(func(fd uintptr) {
		info, serr = unix.GetsockoptTCPInfo(int(fd), unix.IPPROTO_TCP, unix.TCP_INFO)
	}); cerr != nil {
		return 0, 0, cerr
	}
	if serr != nil {
		return 0, 0, serr
	}
	if info == nil {
		return 0, 0, fmt.Errorf("nil TCPInfo")
	}

	return info.Rtt, info.Rttvar, nil
}
