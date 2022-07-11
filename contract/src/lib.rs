//! A minimal hello world smart contract.
extern crate alloc;

use oasis_contract_sdk as sdk;
use oasis_contract_sdk_storage::cell::PublicCell;
use oasis_contract_sdk_types::address::Address;

use sha2::{Sha256};

// Number of seconds from 1. 1. 1970.
type Timestamp = u64;

// First 8 bytes of the hashed value of Sensor.
type SensorID = u64;

// Sequence number of the measurement.
type Seq = u64;

pub enum MeasurementType {
    Temperature = 1,
    Humidity = 2,
    Co2 = 3,
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
const OWNER: &Address = PublicCell::new(b"owner");

/// Stores sensors information.
const SENSORS: PublicMap<SensorID, Sensor> = PublicCell::new(b"sensors");

/// Stores measurements of sensors.
const MEASUREMENTS: PublicMap<(SensorID, MeasurementType, Seq), Vec<(Timestamp, i32)>> = PublicCell::new(b"measurements");

#[derive(Debug, thiserror::Error, sdk::Error)]
pub enum Error {
    #[error("bad request")]
    #[sdk_error(code = 1)]
    BadRequest,

    #[error("forbidden")]
    #[sdk_error(code = 2)]
    Forbidden,
}

#[derive(Clone, Debug, cbor::Encode, cbor::Decode)]
pub enum Request {
    #[cbor(rename = "instantiate")]
    Instantiate { },

    #[cbor(rename = "register_sensor")]
    RegisterSensor { sensor: Sensor },

    #[cbor(rename = "submit_measurements")]
    SubmitMeasurements { sensor_id: SensorID, measurement_type: MeasurementType, measurements: Vec<(Timestamp, i32)> },

    #[cbor(rename = "query_max")]
    QueryMax { sensor_id: SensorID, measurement_type: MeasurementType, start: Timestamp, end: Timestamp },
}

#[derive(Clone, Debug, PartialEq, cbor::Encode, cbor::Decode)]
pub enum Response {
    #[cbor(rename = "register_sensor")]
    RegisterSensor { sensor_id: u64 },

    #[cbor(rename = "query_max")]
    QueryMax { max: i32},

    #[cbor(rename = "empty")]
    Empty,
}

pub struct Cloudy;

impl Cloudy {
    /// Registers a new sensor.
    fn register_sensor<C: sdk::Context>(ctx: &mut C, sensor: Sensor) -> Result<SensorID, Error> {
        let owner = OWNER.get(ctx.public_store());
        if owner != ctx.caller_address() {
            return Err(Error::Forbidden);
        }

        let mut hasher = Sha256::new();
        hasher.update(sensor.name + ctx.caller_address());
        let sensor_id = hasher.finalize()[..8].read_u64::<BigEndian>().unwrap();;
        SENSORS.set(ctx.public_store(), sensor_id, sensor);

        Ok(sensor_id)
    }

    // TODO: Rewrite to macro.
    fn timestamp_floor(granularity: u64, start: Timestamp) -> Timestamp {
        return start - (start%granularity);
    }

    /// Add measurements to MEASUREMENTS. Measurements must be sorted by timestamp and must not exist yet in the contract.
    fn submit_measurements<C: sdk::Context>(ctx: &mut C, sensor_id: SensorID, measurement_type: MeasurementType, measurements: Vec<(Timestamp, i32)>) -> Result<(), Error> {
        if measurements.len() == 0 {
            return Ok(());
        }
        let sensor = SENSORS.get(ctx.public_store(), sensor_id).unwrap();
        let seq = measurements[0].timestamp / sensor.storage_granularity;
        let mut curSeqVals: Vec<(Timestamp, i32)> = MEASUREMENTS.get(ctx.public_store(), (sensor_id, measurement_type, seq)).unwrap_or(vec![]);
        let mut oldSeq = seq;
        while m < measurements.iter() {
            if seq != oldSeq {
                MEASUREMENTS.set(ctx.public_store(), (sensor_id, measurement_type, oldSeq), curSeqVals);
                curSeqVals = MEASUREMENTS.get(ctx.public_store(), (sensor_id, measurement_type, seq)).unwrap_or(vec![]);
                oldSeq = seq;
            }

            curSeqVals.push(m);

            seq = m.timestamp / sensor.storage_granularity;
        }
        if curSeqVals.len() > 0 {
            MEASUREMENTS.set(ctx.public_store(), (sensor_id, measurement_type, seq), curSeqVals);
        }

        Ok(())
    }

    /// Compute the maximum.
    fn compute_max<C: sdk::Context>(ctx: &mut C, sensor_id: SensorID, measurement_type: MeasurementType, start: Timestamp, end: Timestamp) -> i32 {
        let sensor = SENSORS.get(ctx.public_store(), sensor_id).unwrap();
        let ts_start = Self::timestamp_floor(sensor.query_granularity, start);
        let ts_end = Self::timestamp_floor(sensor.query_granularity, end)+sensor.query_granularity;
        let mut seq = Self::timestamp_floor(sensor.storage_granularity, ts_start)/sensor.storage_granularity;
        let mut max_seq = (Self::timestamp_floor(sensor.storage_granularity, ts_end)+sensor.storage_granularity)/sensor.storage_granularity;

        let max_temp: i32 = i32::MIN;
        while seq < max_seq {
            for m in MEASUREMENTS.get(ctx.public_store(), (sensor_id, measurement_type, seq)) {
                if m.1 > max_temp {
                    max_temp = m.1
                }
            }
            seq += 1
        }

        m
    }

}

impl sdk::Contract for Cloudy {
    type Request = Request;
    type Response = Response;
    type Error = Error;

    fn instantiate<C: sdk::Context>(ctx: &mut C, request: Request) -> Result<(), Error> {
        match request {
            Request::Instantiate {} => {
                OWNER.set(ctx.public_store(), ctx.caller_address());
                Ok(())
            }
            _ => Err(Error::BadRequest),
        }
    }

    fn call<C: sdk::Context>(ctx: &mut C, request: Request) -> Result<Response, Error> {
        match request {
            Request::RegisterSensor { sensor } => {
                match Self::register_sensor(ctx, sensor) {
                    Ok(sensor_id) => Ok(Response::RegisterSensor {
                            sensor_id: sensor_id,
                        }),
                    err => err,
                }
            }
            Request::SubmitMeasurements { sensor_id, measurement_type, measurements } => {
                match Self::submit_measurements(ctx, sensor_id, measurement_type, measurements) {
                    Ok(()) => Ok(Response::Empty),
                    Err(e) => Err(e),
                }
            }
            _ => Err(Error::BadRequest),
        }
    }

    fn query<C: sdk::Context>(_ctx: &mut C, _request: Request) -> Result<Response, Error> {
        match request {
            Request::QueryMax { sensor_id, measurement_type, start, end } => {
                match Self::query_max(ctx, sensor_id, measurement_type, start, end) {
                    Ok(max) => Ok(Response::QueryMax {
                        max: max,
                    }),
                    err => err,
                }
            }
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
        Cloudy::instantiate(
            &mut ctx,
            Request::Instantiate {
            },
        )
            .expect("instantiation should work");

        // Register sensor.
        let rsp = Cloudy::call(
            &mut ctx,
            Request::RegisterSensor {
                sensor: Sensor{
                    name: b"esp2866_bedroom",
                    measurement_types: vec![MeasurementType.Temperature, MeasurementType.Humidity],
                    storage_granularity: 10*60, // 10 minutes
                    query_granularity: 3600*4,  // 4 hours
                },
            },
        )
            .expect("RegisterSensor call should work");

        // Make sure sensor_id is correct.
        assert_eq!(
            rsp.sensor_id != 0,
            true
        );

        let sensor_id = rsp.sensor_id;

        // Send some measurements.
        let rsp = Cloudy::call(
            &mut ctx,
            Request::SubmitMeasurements {
                sensor_id: sensor_id,
                measurement_type: MeasurementType::Temperature,
                measurements: vec![(1657541274, 2350), (1657541284, 2360), (1657541294, 2350)],
            },
        )
            .expect("SubmitMeasurements should work");

        // Query for maximum temperature.
        let rsp = Cloudy::call(
            &mut ctx,
            Request::QueryMax {
                sensor_id: sensor_id,
                measurement_type: MeasurementType::Temperature,
                start: 1657540000,
                end: 1657550000,
            },
        )
            .expect("QueryMax should work");

        // Make sure max is correct.
        assert_eq!(
            rsp,
            Response::QueryMax {
                max: 2360,
            }
        );
    }
}
