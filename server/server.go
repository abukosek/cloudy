package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"os"
)

func main() {
	// Load server's private key and certificate.
	crt, err := tls.LoadX509KeyPair("server.crt", "server.key")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Unable to load server key pair: %v\n", err)
		os.Exit(1)
	}

	// Start listening for connections.
	config := &tls.Config{Certificates: []tls.Certificate{crt}}
	server, err := tls.Listen("tcp", ":42424", config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Unable to listen on port 42424: %v\n", err)
		os.Exit(1)
	}
	defer server.Close()

	fmt.Fprintf(os.Stderr, "*** Server is running on port 42424.\n")

	// Accept and handle incoming connections.
	for {
		conn, err := server.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: accept: %v\n", err)
			continue
		}

		go handle(conn)
	}
}

func handle(conn net.Conn) {
	defer conn.Close()

	r := bufio.NewReader(conn)

	for {
		data, err := r.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: read: %v\n", err)
			return
		}

		fmt.Printf("DATA: [%s]\n", data)
	}
}
