# Cloudy smart contract for Cipher ParaTime

To test the contract locally on Intel:

```
cargo test
```

To compile the production contract to Wasm run:

```
cargo build --target wasm32-unknown-unknown --release
```

The contract Wasm will be located in `./target/wasm32-unknown-unknown/release`
folder.

