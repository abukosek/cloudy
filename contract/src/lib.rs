//! A minimal hello world smart contract.
extern crate alloc;

use std::collections::HashMap;

use oasis_contract_sdk as sdk;
use oasis_contract_sdk_storage::{
    cell::PublicCell,
    map::{ConfidentialMap, PublicMap},
};
use oasis_contract_sdk_types::address::Address;

use sha2::{Digest, Sha256};

// Number of seconds from 1. 1. 1970.
type Timestamp = u64;

// First 8 bytes of the hashed value of Sensor.
type SensorID = [u8; 8];

// Sequence number of the measurement.
type Seq = u64;

#[derive(Clone, Copy, Debug, Eq, PartialEq, Hash, cbor::Encode, cbor::Decode)]
#[repr(u16)]
pub enum MeasurementType {
    Temperature = 1,
    Pressure = 2,
    Humidity = 3,
    CO2 = 4,
    Illuminance = 5,
    RSSI = 6,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq, Hash, cbor::Encode, cbor::Decode)]
#[repr(u8)]
pub enum ComputeType {
    Max = 1,
    Min = 2,
    Avg = 3,
}

// Unique key bucket for storing measurements.
type MeasurementKey = [u8; 24];

fn to_measurement_key(id: SensorID, t: MeasurementType, seq: Seq) -> MeasurementKey {
    // XXX: Only equal-size arrays can be concatenated.
    match [id, (t as u64).to_be_bytes(), seq.to_be_bytes()]
        .concat()
        .try_into()
    {
        Ok(r) => r,
        _ => panic!("Failed to concatenate {:?}, {}, {}", id, t as u64, seq),
    }
}

#[derive(Clone, Copy, Debug, cbor::Encode, cbor::Decode)]
pub struct MeasurementValue {
    /// Timestamp of the measurement.
    timestamp: Timestamp,

    // Measurement-specific value.
    value: i32,
}

#[derive(Clone, Debug, PartialEq, cbor::Encode, cbor::Decode)]
pub struct Sensor {
    /// Sensor name. E.g. "esp8266_kitchen".
    name: String,

    /// List of types of measurements.
    measurement_types: Vec<MeasurementType>,

    /// Expected interval between calling submitBatch() in seconds. Used to reduce number of writes to confidential storage.
    storage_granularity: u64,

    /// Minimum allowed granularity in seconds for performing confidential queries. Should be multiply of storage_granularity.
    query_granularity: u64,
}

/// Stores who instantiated the contract.
const OWNER: PublicCell<Address> = PublicCell::new(b"owner");

/// Stores sensors information.
const SENSORS: PublicMap<SensorID, Sensor> = PublicMap::new(b"sensors");

/// Stores measurements of sensors.
const MEASUREMENTS: ConfidentialMap<MeasurementKey, Vec<MeasurementValue>> =
    ConfidentialMap::new(b"measurements");

#[derive(Debug, thiserror::Error, sdk::Error)]
pub enum Error {
    #[error("bad request")]
    #[sdk_error(code = 1)]
    BadRequest,

    #[error("forbidden")]
    #[sdk_error(code = 2)]
    Forbidden,

    #[error("unknown sensor id")]
    #[sdk_error(code = 3)]
    UnknownSensorID,
}

#[derive(Clone, Debug, cbor::Encode, cbor::Decode)]
pub enum Request {
    #[cbor(rename = "instantiate")]
    Instantiate {},

    #[cbor(rename = "register_sensor")]
    RegisterSensor { sensor: Sensor },

    #[cbor(rename = "get_sensors_by_name")]
    GetSensorsByName { sensor_names: Vec<String> },

    #[cbor(rename = "submit_measurements")]
    SubmitMeasurements {
        sensor_id: SensorID,
        measurements: HashMap<MeasurementType, Vec<MeasurementValue>>,
    },

    #[cbor(rename = "query")]
    Query {
        sensor_id: SensorID,
        measurement_type: MeasurementType,
        compute_type: ComputeType,
        start: Timestamp,
        end: Timestamp,
    },
}

#[derive(Clone, Debug, PartialEq, cbor::Encode, cbor::Decode)]
pub enum Response {
    #[cbor(rename = "register_sensor")]
    RegisterSensor { sensor_id: SensorID },

    #[cbor(rename = "get_sensors_by_name")]
    GetSensorsByName { sensors: HashMap<SensorID, Sensor> },

    #[cbor(rename = "query")]
    Query { value: i32 },

    #[cbor(rename = "empty")]
    Empty,
}

pub struct Cloudy;

impl Cloudy {
    /// Registers a new sensor.
    fn register_sensor<C: sdk::Context>(ctx: &mut C, sensor: Sensor) -> Result<SensorID, Error> {
        let caller_address = ctx.caller_address().clone();
        let owner = match OWNER.get(ctx.public_store()) {
            Some(o) => o,
            None => return Err(Error::BadRequest),
        };
        if owner != caller_address {
            return Err(Error::Forbidden);
        }
        let sensor_id = match Self::compute_sensor_id(sensor.name.clone(), ctx.caller_address()) {
            Ok(id) => id,
            Err(_) => return Err(Error::BadRequest),
        };
        SENSORS.insert(ctx.public_store(), sensor_id, sensor);

        Ok(sensor_id)
    }

    /// Returns the sensor for the given sensor name for the caller or None.
    fn get_sensor_by_name<C: sdk::Context>(
        ctx: &mut C,
        name: String,
    ) -> Option<(SensorID, Sensor)> {
        let sensor_id = match Self::compute_sensor_id(name, ctx.caller_address()) {
            Ok(id) => id,
            Err(_) => return None,
        };

        match SENSORS.get(ctx.public_store(), sensor_id) {
            Some(s) => Some((sensor_id, s)),
            None => None,
        }
    }

    // TODO: Rewrite to macro.
    fn compute_sensor_id(
        name: String,
        address: &Address,
    ) -> Result<SensorID, std::array::TryFromSliceError> {
        let mut hasher = Sha256::new();
        hasher.update(name.clone() + &address.to_bech32());
        hasher.finalize()[..8].try_into()
    }

    // TODO: Rewrite to macro.
    fn timestamp_floor(granularity: u64, start: Timestamp) -> Timestamp {
        return start - (start % granularity);
    }

    /// Add measurements to MEASUREMENTS. Measurements must be sorted by timestamp and must not exist yet in the contract.
    fn submit_measurements<C: sdk::Context>(
        ctx: &mut C,
        sensor_id: SensorID,
        measurements: HashMap<MeasurementType, Vec<MeasurementValue>>,
    ) -> Result<(), Error> {
        if measurements.len() == 0 {
            return Ok(());
        }
        let sensor: Sensor = match SENSORS.get(ctx.public_store(), sensor_id) {
            Some(s) => s,
            None => return Err(Error::UnknownSensorID),
        };

        for (measurement_type, measurement_values) in &measurements {
            if !sensor.measurement_types.contains(measurement_type) {
                return Err(Error::BadRequest);
            }

            let mut seq = measurement_values[0].timestamp / sensor.storage_granularity;
            let mut cur_seq_vals: Vec<MeasurementValue> = MEASUREMENTS
                .get(
                    ctx.confidential_store(),
                    to_measurement_key(sensor_id, *measurement_type, seq),
                )
                .unwrap_or(vec![]);
            let mut old_seq = seq;
            for m in measurement_values.iter() {
                if seq != old_seq {
                    MEASUREMENTS.insert(
                        ctx.confidential_store(),
                        to_measurement_key(sensor_id, *measurement_type, old_seq),
                        cur_seq_vals,
                    );
                    cur_seq_vals = MEASUREMENTS
                        .get(
                            ctx.confidential_store(),
                            to_measurement_key(sensor_id, *measurement_type, seq),
                        )
                        .unwrap_or(vec![]);
                    old_seq = seq;
                }

                cur_seq_vals.push(*m);

                seq = m.timestamp / sensor.storage_granularity;
            }
            if cur_seq_vals.len() > 0 {
                MEASUREMENTS.insert(
                    ctx.confidential_store(),
                    to_measurement_key(sensor_id, *measurement_type, seq),
                    cur_seq_vals,
                );
            }
        }

        Ok(())
    }

    /// Compute the maximum, minimum, average.
    fn compute<C: sdk::Context>(
        ctx: &mut C,
        sensor_id: SensorID,
        measurement_type: MeasurementType,
        compute_type: ComputeType,
        start: Timestamp,
        end: Timestamp,
    ) -> i32 {
        let sensor = SENSORS.get(ctx.public_store(), sensor_id).unwrap();
        let ts_start = Self::timestamp_floor(sensor.query_granularity, start);
        let ts_end =
            Self::timestamp_floor(sensor.query_granularity, end) + sensor.query_granularity;
        let mut seq = Self::timestamp_floor(sensor.storage_granularity, ts_start)
            / sensor.storage_granularity;
        let end_seq = (Self::timestamp_floor(sensor.storage_granularity, ts_end)
            + sensor.storage_granularity)
            / sensor.storage_granularity;

        let mut v: i32 = match compute_type {
            ComputeType::Max => i32::MIN,
            ComputeType::Min => i32::MAX,
            _ => 0,
        };
        let mut steps: i32 = 0;
        while seq < end_seq {
            for m in MEASUREMENTS
                .get(
                    ctx.confidential_store(),
                    to_measurement_key(sensor_id, measurement_type, seq),
                )
                .unwrap_or(vec![])
            {
                match compute_type {
                    ComputeType::Max => {
                        if m.value > v {
                            v = m.value
                        }
                    }
                    ComputeType::Min => {
                        if m.value < v {
                            v = m.value
                        }
                    }
                    ComputeType::Avg => v += m.value,
                }
                steps += 1
            }
            seq += 1
        }

        match compute_type {
            ComputeType::Max | ComputeType::Min => v,
            ComputeType::Avg => match steps {
                0 => 0,
                delta => v / delta,
            },
        }
    }
}

impl sdk::Contract for Cloudy {
    type Request = Request;
    type Response = Response;
    type Error = Error;

    fn instantiate<C: sdk::Context>(ctx: &mut C, request: Request) -> Result<(), Error> {
        match request {
            Request::Instantiate {} => {
                let caller_address = ctx.caller_address().clone();
                OWNER.set(ctx.public_store(), caller_address);
                Ok(())
            }
            _ => Err(Error::BadRequest),
        }
    }

    fn call<C: sdk::Context>(ctx: &mut C, request: Request) -> Result<Response, Error> {
        match request {
            Request::RegisterSensor { sensor } => match Self::register_sensor(ctx, sensor) {
                Ok(sensor_id) => Ok(Response::RegisterSensor {
                    sensor_id: sensor_id,
                }),
                Err(err) => Err(err),
            },
            Request::GetSensorsByName { sensor_names } => Ok(Response::GetSensorsByName {
                sensors: HashMap::from_iter(
                    sensor_names
                        .into_iter()
                        .map(|n| Self::get_sensor_by_name(ctx, n))
                        .filter(|s| s.is_some())
                        .map(|s| s.unwrap()),
                ),
            }),
            Request::SubmitMeasurements {
                sensor_id,
                measurements,
            } => match Self::submit_measurements(ctx, sensor_id, measurements) {
                Ok(()) => Ok(Response::Empty),
                Err(e) => Err(e),
            },
            // TODO: Query request handling should reside solely inside Self::query().
            Request::Query {
                sensor_id,
                measurement_type,
                compute_type,
                start,
                end,
            } => Ok(Response::Query {
                value: Self::compute(ctx, sensor_id, measurement_type, compute_type, start, end),
            }),
            _ => Err(Error::BadRequest),
        }
    }

    fn query<C: sdk::Context>(ctx: &mut C, request: Request) -> Result<Response, Error> {
        match request {
            Request::Query {
                sensor_id,
                measurement_type,
                compute_type,
                start,
                end,
            } => Ok(Response::Query {
                value: Self::compute(ctx, sensor_id, measurement_type, compute_type, start, end),
            }),
            _ => Err(Error::BadRequest),
        }
    }
}

sdk::create_contract!(Cloudy);

#[cfg(test)]
mod test {
    use oasis_contract_sdk::{testing::MockContext, types::ExecutionContext, Contract};

    use super::*;

    #[test]
    fn test() {
        // Create a mock execution context with default values.
        let mut ctx: MockContext = ExecutionContext::default().into();

        // Instantiate the contract.
        Cloudy::instantiate(&mut ctx, Request::Instantiate {}).expect("instantiation should work");

        // Register sensor.
        let my_sensor = Sensor {
            name: "esp2866_bedroom".to_string(),
            measurement_types: vec![MeasurementType::Temperature, MeasurementType::Humidity],
            storage_granularity: 10 * 60, // 10 minutes
            query_granularity: 3600 * 4,  // 4 hours
        };
        let rsp = Cloudy::call(
            &mut ctx,
            Request::RegisterSensor {
                sensor: my_sensor.clone(),
            },
        )
        .expect("RegisterSensor call should work");

        let sensor_id = match rsp {
            Response::RegisterSensor { sensor_id } => sensor_id,
            _ => panic!(
                "calling with Request::RegisterSensor does not return Response::ReigsterSensor"
            ),
        };

        // Make sure sensor_id is correct.
        assert_ne!(sensor_id, [0, 0, 0, 0, 0, 0, 0, 0],);

        // Check, if sensor was registered and get some non-existent sensor name.
        let rsp = Cloudy::call(
            &mut ctx,
            Request::GetSensorsByName {
                sensor_names: vec![
                    "esp2866_bedroom".to_string(),
                    "some-non-existent sensor".to_string(),
                ],
            },
        )
        .expect("GetSensorsByName should work");

        // Make sure list of sensors is correct.
        assert_eq!(
            rsp,
            Response::GetSensorsByName {
                sensors: HashMap::from([(sensor_id, my_sensor)]),
            }
        );

        // Send some measurements.
        let req = Request::SubmitMeasurements {
            sensor_id: sensor_id,
            measurements: HashMap::from([(
                MeasurementType::Temperature,
                vec![
                    MeasurementValue {
                        timestamp: 1657541274,
                        value: 2350,
                    },
                    MeasurementValue {
                        timestamp: 1657541284,
                        value: 2360,
                    },
                    MeasurementValue {
                        timestamp: 1657541294,
                        value: 2340,
                    },
                ],
            )]),
        };
        Cloudy::call(&mut ctx, req).expect("SubmitMeasurements should work");

        // Query for maximum temperature.
        let rsp = Cloudy::query(
            &mut ctx,
            Request::Query {
                sensor_id: sensor_id,
                measurement_type: MeasurementType::Temperature,
                compute_type: ComputeType::Max,
                start: 1657540000,
                end: 1657550000,
            },
        )
        .expect("Query for maximum should work");
        assert_eq!(rsp, Response::Query { value: 2360 });

        // Query for minimum temperature.
        let rsp = Cloudy::query(
            &mut ctx,
            Request::Query {
                sensor_id: sensor_id,
                measurement_type: MeasurementType::Temperature,
                compute_type: ComputeType::Min,
                start: 1657540000,
                end: 1657550000,
            },
        )
        .expect("Query for minimum should work");
        assert_eq!(rsp, Response::Query { value: 2340 });

        // Query for average temperature.
        let rsp = Cloudy::query(
            &mut ctx,
            Request::Query {
                sensor_id: sensor_id,
                measurement_type: MeasurementType::Temperature,
                compute_type: ComputeType::Avg,
                start: 1657540000,
                end: 1657550000,
            },
        )
        .expect("Query for average should work");
        assert_eq!(rsp, Response::Query { value: 2350 });
    }
}
