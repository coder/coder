package main

// A simple echo server.  It listens on a random port, prints that port, then
// echos back anything sent to it.

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
)

func main() {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("listen error: err=%s", err)
	}

	defer l.Close()
	tcpAddr, valid := l.Addr().(*net.TCPAddr)
	if !valid {
		log.Fatal("address is not valid")
	}

	remotePort := tcpAddr.Port
	_, err = fmt.Println(remotePort)
	if err != nil {
		log.Fatalf("print error: err=%s", err)
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Fatalf("accept error, err=%s", err)
			return
		}

		go func() {
			defer conn.Close()
			_, err := io.Copy(conn, conn)

			if errors.Is(err, io.EOF) {
				return
			} else if err != nil {
				log.Fatalf("copy error, err=%s", err)
			}
		}()
	}
}
