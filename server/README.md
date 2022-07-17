# Server

This directory contains the code for the Go server that collects sensor data
from the sensors, batches it, and submits it to the Cipher smart contract.

## Running Cipher client e2e tests

Prerequisites:
- Download and extract Oasis Core 22.1.8 to `/oasis_core`,
- Download [Cipher paratime] source to `/cipher-paratime` and compile it in
  release mode `cargo build --release`,
- Download [Oasis CLI], compile it with `go build -o oasis` and add `oasis` to
  your path.

To run Cipher e2e tests locally:

1. Spin up Cipher locally with prepopulated Alice's account by running:
   ```shell
   rm -rf /tmp/cipher-test; mkdir -p /tmp/cipher-test
   /oasis-core/oasis-net-runner \
     --fixture.default.node.binary /oasis-core/oasis-node \
     --fixture.default.runtime.binary /cipher-paratime/target/release/cipher-paratime \
     --fixture.default.runtime.loader /oasis-core/oasis-core-runtime-loader \
     --fixture.default.runtime.provisioner unconfined \
     --fixture.default.keymanager.binary '' \
     --fixture.default.runtime.version '2.5.0' \
     --basedir /tmp/cipher-test \
     --basedir.no_temp_dir
   ```
2. Register runtime locally with oasis CLI:
   ```shell
   oasis network add-local localhost unix:/tmp/cipher-test/net-runner/network/client-0/internal.sock
   oasis paratime add localhost cipher 8000000000000000000000000000000000000000000000000000000000000000
   oasis inspect node-status --network localhost # should see "latest_round" increasing in a few moments
   ```
3. Compile the smart contract by moving into `contract` folder and running:
   ```shell
   cargo build --target wasm32-unknown-unknown --release
   ```
4. Upload the smart contract and instantiate it:
   ```shell
   oasis contracts upload ./target/wasm32-unknown-unknown/release/cloudy.wasm --account test:alice --network localhost --gas-limit 3000000
   # Code ID should be printed in a while.
   oasis contracts instantiate 0 "instantiate: {}" --account test:alice --network localhost --gas-limit 100000
   # Instance ID should be printed in a while.
   ```
5. Run the tests with
   ```shell
   go test ./...
   ```

[Cipher paratime]: https://github.com/oasisprotocol/cipher-paratime
[Oasis CLI]: https://github.com/oasisprotocol/oasis-sdk/tree/main/cli
