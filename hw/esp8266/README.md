# ESP8266

This directory contains the code for reporting sensor data on the ESP8266
platform running [NodeMCU][0].

## How to use

1. Flash the NodeMCU firmware onto your ESP866 board.
2. Set-up the configuration.
3. Upload the code and configuration to the ESP8266 board.
4. Restart the board and enjoy.

### Flashing NodeMCU

You can use the pre-built firmware file included in this repository
(`nodemcu-release-14-modules-2022-07-14-12-14-56-integer.bin`) or use the
[NodeMCU Build system][1] or build your own from scratch.

You will need to install [esptool][2] first.  The version should be 3.2 or
higher (the one included in the Ubuntu repository is too old, don't use that).

The firmware needs to be flashed only once.  First erase the flash with:

	sudo esptool -p /dev/ttyUSB0 erase_flash

Then flash the NodeMCU firmware with:

	sudo esptool -p /dev/ttyUSB0 write_flash 0x00000 nodemcu-release-14-modules-2022-07-14-12-14-56-integer.bin

Restart the board and continue with the rest of the steps.

### Configuring

Copy `config.example.lua` to `config.lua` and follow the instructions inside
the file.

You will need to specify the sensor's unique name, your WiFi network's SSID and
password, the address of your NTP server, the address and port of the Go server,
as well as its server certificate.  Finally, you can configure which sensors
you have attached to the board.

### Uploading code and config

To upload the code and configuration, you will need `nodemcu-uploader`.
Install it with:

	sudo pip install -U --system nodemcu-uploader

Then do the upload with:

	sudo nodemcu-uploader --port=/dev/ttyUSB0 upload config.lua
	sudo nodemcu-uploader --port=/dev/ttyUSB0 upload init.lua

Sometimes the upload timing is a bit tricky.  If you get failures, restart the
board, wait exactly 1 second for the blue LED to go off, and then retry.

Once you have uploaded the configuration and code, restart the board and monitor
the output with:

	screen /dev/ttyUSB0 115200


[0]: https://nodemcu.readthedocs.io/en/release/
[1]: https://nodemcu-build.com/
[2]: https://github.com/espressif/esptool/releases

