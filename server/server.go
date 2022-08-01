package main

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"time"

	"gopkg.in/yaml.v3"
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
	Co2         uint16 `json:"Co2,omitempty"` // ppm
	Illuminance uint16 `json:"Ev,omitempty"`  // lux
}

type SensorDatabase struct {
	Sensors []string
}

var (
	// Sensor database.
	SensorDB SensorDatabase

	// Map of sensor name -> ID.
	KnownSensors map[string]SensorID
)

func main() {
	// Load server's private key and certificate.
	crt, err := tls.LoadX509KeyPair("server.crt", "server.key")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Unable to load server key pair: %v\n", err)
		os.Exit(1)
	}

	// Load sensor database.
	sensorDBRaw, err := ioutil.ReadFile("sensor-db.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Unable to read sensor database: %v\n", err)
		os.Exit(1)
	}
	err = yaml.Unmarshal(sensorDBRaw, &SensorDB)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Unable to parse sensor database: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "*** Sensor database contains %d sensor(s).\n", len(SensorDB.Sensors))

	// Check if the sensors are already registered, otherwise register them.
	// Also obtain sensor IDs and set-up a map of sensor name to ID.
	for _, sensor := range SensorDB.Sensors {
		// TODO: matevz: Register if needed and obtain sensor ID.
		KnownSensors[sensor] = SensorID{}
	}

	// TODO: When client authentication is implemented, also verify if the
	// sensor certificates match those stored on the blockchain.

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

			if _, ok := KnownSensors[d.Name]; !ok {
				// Discard data from sensor that's not in our DB.
				fmt.Fprintf(os.Stderr, "*** Ignoring data from sensor not configured in sensor DB: %s\n", d.Name)
				continue
			}

			// Batch data for sending.
			batch = append(batch, d)
		case <-submitBatchTicker.C:
			// Only submit batch if there's anything in it.
			if len(batch) == 0 {
				// TODO: Maybe also add a minimum batch size?
				continue
			}

			fmt.Fprintf(os.Stderr, "*** Submitting batch with %d sensor readings.\n", len(batch))

			// Submit batch.
			if err := convertBatchAndSubmit(batch); err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: Failed to submit batch: %v\n", err)
				continue
				// Leave the contents of the batch for next time.
			}

			// Reset batch if submission was successful.
			batch = make([]SensorData, 0)
		}
	}
}

func convertBatchAndSubmit(batch []SensorData) error {
	// Sensor name -> SubmitMeasurementsRequest.
	sensors := make(map[string]SubmitMeasurementsRequest)

	for _, b := range batch {
		sensor, exists := sensors[b.Name]
		if !exists {
			sensors[b.Name] = SubmitMeasurementsRequest{
				SensorID:     KnownSensors[b.Name],
				Measurements: make(map[MeasurementType][]MeasurementValue),
			}
			sensor = sensors[b.Name]
		}

		sensor.Measurements[RSSI] = append(sensor.Measurements[RSSI], MeasurementValue{
			Timestamp: Timestamp(b.Timestamp),
			Value:     int32(b.RSSI),
		})

		if b.Pressure > 0 {
			sensor.Measurements[Pressure] = append(sensor.Measurements[Pressure], MeasurementValue{
				Timestamp: Timestamp(b.Timestamp),
				Value:     int32(b.Pressure),
			})

			// If we have a valid pressure reading, we also have a valid
			// temperature reading.
			sensor.Measurements[Temperature] = append(sensor.Measurements[Temperature], MeasurementValue{
				Timestamp: Timestamp(b.Timestamp),
				Value:     b.Temperature,
			})
		}

		if b.Humidity > 0 {
			sensor.Measurements[Humidity] = append(sensor.Measurements[Humidity], MeasurementValue{
				Timestamp: Timestamp(b.Timestamp),
				Value:     int32(b.Humidity),
			})
		}

		if b.Co2 > 0 {
			sensor.Measurements[Co2] = append(sensor.Measurements[Co2], MeasurementValue{
				Timestamp: Timestamp(b.Timestamp),
				Value:     int32(b.Co2),
			})
		}

		if b.Illuminance > 0 {
			sensor.Measurements[Illuminance] = append(sensor.Measurements[Illuminance], MeasurementValue{
				Timestamp: Timestamp(b.Timestamp),
				Value:     int32(b.Illuminance),
			})
		}
	}

	// Debug.
	j, _ := json.Marshal(sensors)
	fmt.Printf("PREPARED BATCH: %s\n", j)

	// Now submit the requests.
	for _, s := range sensors {
		// TODO: matevz.
		_ = s
	}

	return nil
}
