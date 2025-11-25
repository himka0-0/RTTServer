package echo

import (
	"log"
	"net"
	"time"
)

func StartEchoFiltered(addr string, firstByteTimeout time.Duration, fallback func(net.Conn)) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("listen %s: %v", addr, err)
		return
	}
	log.Printf("Mux listening on %s", addr)

	for {
		c, err := ln.Accept()
		if err != nil {
			log.Printf("accept %s: %v", addr, err)
			continue
		}
		go func(conn net.Conn) {
			_ = conn.SetReadDeadline(time.Now().Add(firstByteTimeout))
			var b [1]byte
			n, err := conn.Read(b[:])

			if (err != nil && isTimeout(err)) || n == 0 {
				_ = conn.SetDeadline(time.Time{})
				fallback(conn)
				return
			}

			if err == nil && n == 1 && b[0] == 0xAA {
				_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
				_, _ = conn.Write(b[:])
				conn.Close()
				return
			}

			conn.Close()
		}(c)
	}
}

func isTimeout(err error) bool {
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return true
	}
	return false
}
