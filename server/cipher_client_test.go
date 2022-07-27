package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/oasisprotocol/oasis-core/go/common"
	"github.com/oasisprotocol/oasis-core/go/common/cbor"
	cmnGrpc "github.com/oasisprotocol/oasis-core/go/common/grpc"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/contracts"
	sdkTesting "github.com/oasisprotocol/oasis-sdk/client-sdk/go/testing"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	// Path to local socket. Replace with , if needed.
	socket = "unix:/tmp/cipher-test/net-runner/network/client-0/internal.sock"
	// socket = "testnet.grpc.oasis.dev:443"
	// Cipher runtime ID.
	runtimeID = "8000000000000000000000000000000000000000000000000000000000000000"
	// First deployed contract.
	instanceID = 0
	// Signer.
	signer = sdkTesting.Alice
)

// E2E test. Requires already instantiated cloudy smart contract.
func TestRegisterSensorSubmitMeasurementsAndQueryMax(t *testing.T) {
	require := require.New(t)

	var rtID common.Namespace
	if err := rtID.UnmarshalHex(runtimeID); err != nil {
		panic(fmt.Sprintf("can't decode runtime ID: %s", err))
	}

	conn, err := cmnGrpc.Dial(socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(fmt.Sprintf("can't connect to socket: %s", err))
	}
	defer conn.Close()

	rtc := client.New(conn, rtID)
	cc := contracts.NewV1(rtc)

	ctx, cancelFn := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancelFn()

	// Test registering new sensor and obtaining new ID.
	req := Request{
		RegisterSensor: &RegisterSensorRequest{
			Sensor: Sensor{
				Name:               "esp2866_bedroom",
				MeasurementTypes:   []MeasurementType{Temperature, Humidity},
				StorageGranularity: 60 * 10,
				QueryGranularity:   3600 * 4,
			},
		},
	}
	txb := cc.Call(contracts.InstanceID(instanceID), req, []types.BaseUnits{})
	result, err := SignAndSubmitTx(ctx, rtc, sdkTesting.Alice.Signer, *txb.GetTransaction(), 200000)
	if err != nil {
		panic(fmt.Sprintf("can't call register_sensor: %s", err))
	}
	var sensorID SensorID
	if err = cbor.Unmarshal(result, &sensorID); err != nil {
		panic(fmt.Sprintf("can't decode register_sensor_response: %s", err))
	}
	require.NotEmpty(sensorID, "sensor ID must not be empty")

	// Test submit temperature measurements.
	req = Request{
		SubmitMeasurements: &SubmitMeasurementsRequest{
			SensorID: sensorID,
			Measurements: map[MeasurementType][]MeasurementValue{
				Temperature: {
					{Timestamp: 1657541274, Value: 2350},
					{Timestamp: 1657541284, Value: 2360},
					{Timestamp: 1657541294, Value: 2350},
				},
			},
		},
	}
	txb = cc.Call(contracts.InstanceID(instanceID), req, []types.BaseUnits{})
	_, err = SignAndSubmitTx(ctx, rtc, sdkTesting.Alice.Signer, *txb.GetTransaction(), 200000)
	if err != nil {
		panic(fmt.Sprintf("can't call submit_measurements: %s", err))
	}

	// Test querying max temperature. Should be 23.60 degC.
	req = Request{
		QueryMax: &QueryMaxRequest{
			SensorID:        sensorID,
			MeasurementType: Temperature,
			Start:           1657540000,
			End:             1657550000,
		},
	}
	txb = cc.Call(contracts.InstanceID(instanceID), req, []types.BaseUnits{})
	result, err = SignAndSubmitTx(ctx, rtc, sdkTesting.Alice.Signer, *txb.GetTransaction(), 200000)
	if err != nil {
		panic(fmt.Sprintf("can't call query_max: %s", err))
	}
	var maxTemp int32
	if err = cbor.Unmarshal(result, &maxTemp); err != nil {
		panic(fmt.Sprintf("can't decode query_max_response: %s", err))
	}
	require.Equal(2360, maxTemp, "maximum temperature must match")
}
