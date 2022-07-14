-- Configuration file.
--
-- Change the settings and rename the file to config.lua, then upload it to the
-- ESP8266 board to make things work.

return {
	--
	-- Sensor identification.
	--

	-- Unique sensor name.
	SENSOR_NAME = "test_sensor",

	-- TLS certificate for sensor authentication.
	-- TODO: CLIENT_CERT = [[]],

	-- Sensor's private key.
	-- TODO: CLIENT_PRIVATE_KEY = [[]],



	--
	-- Network setup.
	--

	-- SSID and password of the WiFi access point to connect to.
	WIFI_SSID = "WIFI_SSID",
	WIFI_PASS = "WIFI_PASS",

	-- Address of the NTP server to use for time synchronization.
	NTP_ADDR = "pool.ntp.org",

	-- Address and port of our Go server.
	SERVER_ADDR = "10.1.1.123",
	SERVER_PORT = 42424,

	-- TLS certificate of our Go server.
	SERVER_CERT = [[
ENTER CERTIFICATE HERE
]],



	--
	-- Attached sensor configuration.
	--

	-- BME280 T/RH/p module connected to 3V3, G, D5 (SCL), D6 (SDA).
	HAVE_BME280 = true,

	-- MH-Z19B IR CO2 sensor (0-5000 ppm) connected to 5V (VU), G, D1 (PWM).
	HAVE_CO2 = true,

	-- BH1750 lux sensor module connected to 3V3, G, D7 (SCL), D2 (SDA).
	HAVE_BH1750 = true
}

