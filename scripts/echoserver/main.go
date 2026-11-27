package main

// A simple echo server.  It listens on a random port, prints that port, then
// echos back anything sent to it.  It exits when stdin is closed so that it
// does not outlive the process that started it.

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
)

func main() {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("listen error: err=%s", err)
	}

	defer l.Close()
	tcpAddr, valid := l.Addr().(*net.TCPAddr)
	if !valid {
		log.Panic("address is not valid")
	}

	remotePort := tcpAddr.Port
	_, err = fmt.Println(remotePort)
	if err != nil {
		log.Panicf("print error: err=%s", err)
	}

	go func() {
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}()

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Panicf("accept error, err=%s", err)
			return
		}

		go func() {
			defer conn.Close()
			_, err := io.Copy(conn, conn)

			if errors.Is(err, io.EOF) {
				return
			} else if err != nil {
				log.Panicf("copy error, err=%s", err)
			}
		}()
	}
}
