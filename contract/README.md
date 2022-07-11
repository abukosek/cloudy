# Cipher smart contract

To test the contract on Intel run:

```
cargo test
```

To compile the production contract to Wasm run:

```
cargo build --target wasm32-unknown-unknown --release
```

The contract Wasm will be located in `./target/wasm32-unknown-unknown/release`
folder.
