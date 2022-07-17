package main

const (
	Temperature MeasurementType = 1
	Humidity    MeasurementType = 2
	Co2         MeasurementType = 3
)

type MeasurementType uint16
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
	SubmitMeasurements *SubmitMeasurementsRequest `json:"submit_measurements,omitempty"`
	QueryMax           *QueryMaxRequest           `json:"query_max,omitempty"`
}

type Response struct {
	RegisterSensor *RegisterSensorResponse `json:"register_sensor,omitempty"`
	QueryMax       *QueryMaxResponse       `json:"query_max,omitempty"`
}

type RegisterSensorRequest struct {
	Sensor Sensor `json:"sensor"`
}

type RegisterSensorResponse struct {
	SensorID SensorID `json:"sensor_id"`
}

type SubmitMeasurementsRequest struct {
	SensorID     SensorID                               `json:"sensor_id"`
	Measurements map[MeasurementType][]MeasurementValue `json:"measurements"`
}

type QueryMaxRequest struct {
	SensorID        SensorID        `json:"sensor_id"`
	MeasurementType MeasurementType `json:"measurement_type"`
	Start           Timestamp       `json:"start"`
	End             Timestamp       `json:"end"`
}

type QueryMaxResponse struct {
	Max uint32 `json:"max"`
}

type MeasurementValue struct {
	_ struct{} `json:",toarray"`

	Timestamp Timestamp
	Value     uint32
}
