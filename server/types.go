package main

import "encoding/hex"

const (
	Temperature MeasurementType = 1
	Pressure    MeasurementType = 2
	Humidity    MeasurementType = 3
	CO2         MeasurementType = 4
	Illuminance MeasurementType = 5
	RSSI        MeasurementType = 6

	Max ComputeType = 1
	Min ComputeType = 2
	Avg ComputeType = 3
)

var (
	EmptyResponseRaw, _ = hex.DecodeString("65656d707479")
)

type MeasurementType uint16
type ComputeType uint8
type SensorID [8]byte
type Timestamp uint64

type Sensor struct {
	Name               string            `json:"name"`
	MeasurementTypes   []MeasurementType `json:"measurement_types"`
	StorageGranularity uint64            `json:"storage_granularity"`
	QueryGranularity   uint64            `json:"query_granularity"`
}

type Request struct {
	RegisterSensor     *RegisterSensorRequest     `json:"register_sensor,omitempty"`
	GetSensorsByName   *GetSensorsByNameRequest   `json:"get_sensors_by_name,omitempty"`
	SubmitMeasurements *SubmitMeasurementsRequest `json:"submit_measurements,omitempty"`
	Query              *QueryRequest              `json:"query,omitempty"`
}

type Response struct {
	RegisterSensor   *RegisterSensorResponse   `json:"register_sensor,omitempty"`
	GetSensorsByName *GetSensorsByNameResponse `json:"get_sensors_by_name,omitempty"`
	Query            *QueryResponse            `json:"query,omitempty"`
	Empty            *EmptyResponse            `json:"empty,omitempty"`
}

type RegisterSensorRequest struct {
	Sensor Sensor `json:"sensor"`
}

type RegisterSensorResponse struct {
	SensorID SensorID `json:"sensor_id"`
}

type GetSensorsByNameRequest struct {
	SensorNames []string `json:"sensor_names"`
}

type GetSensorsByNameResponse struct {
	Sensors map[SensorID]Sensor `json:"sensors"`
}

type SubmitMeasurementsRequest struct {
	SensorID     SensorID                               `json:"sensor_id"`
	Measurements map[MeasurementType][]MeasurementValue `json:"measurements"`
}

type QueryRequest struct {
	SensorID        SensorID        `json:"sensor_id"`
	MeasurementType MeasurementType `json:"measurement_type"`
	ComputeType     ComputeType     `json:"compute_type"`
	Start           Timestamp       `json:"start"`
	End             Timestamp       `json:"end"`
}

type QueryResponse struct {
	Value int32 `json:"value"`
}

type MeasurementValue struct {
	Timestamp Timestamp `json:"timestamp"`
	Value     int32     `json:"value"`
}

type EmptyResponse struct {
}
