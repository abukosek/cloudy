package main

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"
)

// How often to submit batch of sensor readings to the blockchain?
const BatchTime = 10 * time.Minute

type SensorData struct {
	Name        string `json:"name"`
	Timestamp   uint64 `json:"t"`             // UNIX time
	RSSI        int8   `json:"RSSI"`          // dBm
	Temperature int32  `json:"T,omitempty"`   // C*100
	Pressure    uint32 `json:"p,omitempty"`   // hPa*1000
	Humidity    uint32 `json:"RH,omitempty"`  // RH%*1000
	CO2         uint16 `json:"CO2,omitempty"` // ppm
	Illuminance uint16 `json:"Ev,omitempty"`  // lux
}

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

	// Spawn batch handler.
	dataCh := make(chan string)
	go batchHandler(dataCh)

	// Accept and handle incoming connections.
	// TODO: Add client authentication.
	for {
		conn, err := server.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: accept: %v\n", err)
			continue
		}

		go handleConnection(conn, dataCh)
	}
}

func handleConnection(conn net.Conn, dataCh chan string) {
	defer conn.Close()

	r := bufio.NewReader(conn)

	for {
		data, err := r.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: read: %v\n", err)
			return
		}

		fmt.Printf("DATA: [%s]\n", data)

		dataCh <- data
	}
}

func batchHandler(dataCh <-chan string) {
	batch := make([]SensorData, 0)
	submitBatchTicker := time.NewTicker(BatchTime)

	for {
		select {
		case data := <-dataCh:
			// Decode JSON.
			var d SensorData
			if err := json.Unmarshal([]byte(data), &d); err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: malformed data: %v\n", err)
				continue
			}

			// Batch for sending.
			batch = append(batch, d)
		case <-submitBatchTicker.C:
			if len(batch) == 0 {
				// TODO: Maybe also add a minimum batch size?
				continue
			}

			fmt.Fprintf(os.Stderr, "*** Submitting batch with %d sensor readings.\n", len(batch))

			// Submit batch and reset.
			// TODO: Submit.
			batch = make([]SensorData, 0)
		}
	}
}
