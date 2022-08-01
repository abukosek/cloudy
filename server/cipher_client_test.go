package main

import (
	"context"
	"testing"
	"time"

	"github.com/oasisprotocol/oasis-core/go/common"
	cmnGrpc "github.com/oasisprotocol/oasis-core/go/common/grpc"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/contracts"
	sdkTesting "github.com/oasisprotocol/oasis-sdk/client-sdk/go/testing"

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
	instanceID = contracts.InstanceID(0)
	// Signer.
	signer = sdkTesting.Alice.Signer
)

// E2E test. Requires already instantiated cloudy smart contract.
func TestRegisterSensorSubmitMeasurementsAndQueryMax(t *testing.T) {
	require := require.New(t)

	var rtID common.Namespace
	err := rtID.UnmarshalHex(runtimeID)
	require.NoError(err, "runtime ID decoding should succeed")

	conn, err := cmnGrpc.Dial(socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(err, "connection to socket should succeed")
	defer conn.Close()

	rtc := client.New(conn, rtID)

	ctx, cancelFn := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancelFn()

	// Test registering new sensor and obtaining new ID.
	mySensor := Sensor{
		Name:               "esp2866_bedroom",
		MeasurementTypes:   []MeasurementType{Temperature, Humidity},
		StorageGranularity: 60 * 10,
		QueryGranularity:   3600 * 4,
	}
	req := Request{
		RegisterSensor: &RegisterSensorRequest{
			Sensor: mySensor,
		},
	}
	result, err := SignAndSubmitTx(ctx, rtc, signer, req, instanceID)
	require.NoError(err, "register_sensor should succeed")
	require.NotEmpty(result.RegisterSensor, "result.register_sensor must not be empty")
	sensorID := result.RegisterSensor.SensorID
	require.NotEmpty(sensorID, "sensor ID must not be empty")

	req = Request{
		GetSensorsByName: &GetSensorsByNameRequest{
			SensorNames: []string{mySensor.Name, "some-non-existent sensor"},
		},
	}
	result, err = SignAndSubmitTx(ctx, rtc, signer, req, instanceID)
	require.NoError(err, "get_sensors_by_name should succeed")
	require.NotEmpty(result.GetSensorsByName, "result.get_sensors_by_name must not be empty")
	require.Equal(len(result.GetSensorsByName.Sensors), 1, "result.get_sensors_by_name.sensors must have 1 element")
	require.Equal(result.GetSensorsByName.Sensors[sensorID], mySensor)

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
	result, err = SignAndSubmitTx(ctx, rtc, signer, req, instanceID)
	require.NoError(err, "submit_measurements should succeed")

	// Test querying max temperature. Should be 23.60 degC.
	req = Request{
		QueryMax: &QueryMaxRequest{
			SensorID:        sensorID,
			MeasurementType: Temperature,
			Start:           1657540000,
			End:             1657550000,
		},
	}
	result, err = SignAndSubmitTx(ctx, rtc, signer, req, instanceID)
	require.NoError(err, "query_max should succeed")
	require.Equal(int32(2360), result.QueryMax.Max, "maximum temperature must match")
}
