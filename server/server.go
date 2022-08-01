package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"time"

	cmnGrpc "github.com/oasisprotocol/oasis-core/go/common/grpc"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/crypto/signature"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/crypto/signature/ed25519"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"
)

// How often to submit batch of sensor readings to the blockchain?
const BatchTime = 10 * time.Minute

var (
	// Cfg is the config for running a server or performing queries.
	Cfg Config

	// signer is the signer instance used to sign Oasis transactions.
	signer signature.Signer

	rtc client.RuntimeClient

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

	// Load config with sensor database etc.
	cfgRaw, err := ioutil.ReadFile("config.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Unable to read config: %v\n", err)
		os.Exit(1)
	}
	err = yaml.Unmarshal(cfgRaw, &Cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Unable to parse config: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "*** Config contains %d sensor(s).\n", len(Cfg.Sensors))

	conn, err := cmnGrpc.Dial(Cfg.Socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Unable to connect to testSocket %s: %v\n", Cfg.Socket, err)
		os.Exit(1)
	}
	defer conn.Close()

	rtc = client.New(conn, Cfg.RuntimeID)
	ctx, cancelFn := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancelFn()

	var rawSigner ed25519rawSigner
	if err := rawSigner.unmarshalBase64(Cfg.SignerKey); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Unable to parse Signer's private key: %v\n", err)
		os.Exit(1)
	}
	signer = ed25519.WrapSigner(&rawSigner)

	// Check if the sensors are already registered, otherwise register them.
	// Also obtain sensor IDs and set-up a map of sensor name to ID.
	KnownSensors = make(map[string]SensorID)
	sensorNames := []string{}
	for _, sensor := range Cfg.Sensors {
		sensorNames = append(sensorNames, sensor.Name)
	}
	req := Request{
		GetSensorsByName: &GetSensorsByNameRequest{
			SensorNames: sensorNames,
		},
	}
	result, err := SignAndSubmitTx(ctx, rtc, signer, req, Cfg.InstanceID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Unable to sign and submit transaction: %v\n", err)
		os.Exit(1)
	}
	for id, s := range result.GetSensorsByName.Sensors {
		KnownSensors[s.Name] = id
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

		go handleSensorConnection(conn, dataCh)
	}
}

func handleSensorConnection(conn net.Conn, dataCh chan string) {
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
	batch := make([]RawSensorData, 0)
	submitBatchTicker := time.NewTicker(BatchTime)

	for {
		select {
		case data := <-dataCh:
			// Decode JSON.
			var d RawSensorData
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
			batch = make([]RawSensorData, 0)
		}
	}
}

func convertBatchAndSubmit(batch []RawSensorData) error {
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

		if b.CO2 > 0 {
			sensor.Measurements[CO2] = append(sensor.Measurements[CO2], MeasurementValue{
				Timestamp: Timestamp(b.Timestamp),
				Value:     int32(b.CO2),
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
		req := Request{
			SubmitMeasurements: &SubmitMeasurementsRequest{
				SensorID:     s.SensorID,
				Measurements: s.Measurements,
			},
		}
		if _, err := SignAndSubmitTx(context.Background(), rtc, signer, req, Cfg.InstanceID); err != nil {
			return err
		}
	}

	return nil
}
