-- NodeMCU firmware should include the following modules:
--     bme280, cron, file, gpio, i2c, net, node, rtctime, sjson, sntp,
--     tls, tmr, uart, wifi
--
-- To flash the firmware (do this just once):
-- sudo esptool -p /dev/ttyUSB0 erase_flash
-- sudo esptool -p /dev/ttyUSB0 write_flash 0x00000 nodemcu-release-14-modules-2022-07-14-12-14-56-integer.bin
--
-- To flash the code and config:
-- sudo pip install -U --system nodemcu-uploader
-- sudo nodemcu-uploader --port=/dev/ttyUSB0 upload config.lua
-- sudo nodemcu-uploader --port=/dev/ttyUSB0 upload init.lua
--
-- To watch the debug output once the board is powered-on:
-- screen /dev/ttyUSB0 115200

local config = require("config")

local SRV = nil
local SRV_connected = false
local SRV_reconnecting = false

local CO2_ppm = 0
local CO2_lo = 0
local CO2_hi = 0
local CO2_last_t = 0

function CO2_int_handler(level, t)
	if (level == gpio.LOW) then
		CO2_hi = t - CO2_last_t
	else
		CO2_lo = t - CO2_last_t
		CO2_ppm = (5000 * (CO2_hi - 2000)) / (CO2_hi + CO2_lo - 4000)
	end
	CO2_last_t = t
end

function sensor_setup()
	if config.HAVE_BME280 then
		-- BME280 module connected directly to 3V3, G, D5, D6.
		local sda, scl = 6, 5
		i2c.setup(0, sda, scl, i2c.SLOW)
		local bm_type = bme280.setup()
		-- The sensor is set up after about 120ms, but we don't need a delay here
		-- because the first reading is done after the wifi finishes connecting,
		-- which takes longer :)

		if     bm_type == 1 then
			print("[SENS] Detected BMP280 sensor (temperature, pressure).")
		elseif bm_type == 2 then
			print("[SENS] Detected BME280 sensor (temperature, pressure, humidity).")
		else
			print("[SENS] Detected neither BMP280 nor BME280!")
			config.HAVE_BME280 = false
		end
	end

	if config.HAVE_CO2 then
		-- MH-Z19B IR CO2 sensor (0-5000 ppm) connected to 5V (VU), G, D1.
		print("[SENS] Waiting for CO2 sensor to warm up...")
		tmr.create():alarm(120000, tmr.ALARM_SINGLE,
			function(t)
				-- After the CO2 sensor has warmed up (3 min), start measuring PWM.
				CO2_ppm = 0
				CO2_lo = 0
				CO2_hi = 0
				CO2_last_t = tmr.now()
				gpio.mode(1, gpio.INT)
				gpio.trig(1, "both", CO2_int_handler)
				print("[SENS] CO2 sensor warmed up.")
			end)
	end

	if config.HAVE_BH1750 then
		-- BH1750 lux sensor module connected to 3V3, G, D7 (SCL), D2 (SDA).
		-- local sda, scl = 2, 7
		-- i2c.setup(1, sda, scl, i2c.SLOW)

		-- XXX: The firmware from NodeMCU-Builder seems to be built with the
		-- old I2C implementation that only has 1 bus, so this breaks here
		-- with error "i2c 1 does not exist", ugh.  There doesn't seem to be
		-- any option to set it to use the new I2C implementation without
		-- building it ourselves, so let's do an ugly workaround instead.

		print("[SENS] BH1750 illuminance sensor configured.")
	end
end

function sensor_report(_cron_entry)
	-- Get the current timestamp.
	local t_s, t_us, _clk_rate_offset = rtctime.get()

	-- Get signal strength.
	local rssi = wifi.sta.getrssi() -- dBm

	-- Assemble sensor readings.
	local data = {
		name = config.SENSOR_NAME,
		t = t_s,
		RSSI = rssi
	}

	if config.HAVE_BME280 then
		-- XXX: Kludge: Do the I2C setup again (see comment in sensor_setup).
		local bme_sda, bme_scl = 6, 5
		i2c.setup(0, bme_sda, bme_scl, i2c.SLOW)

		local T, p, RH = bme280.read() -- C*100, hPa*1000, RH%*1000
		data.T = T
		data.p = p
		data.RH = RH
	end

	if config.HAVE_CO2 then
		-- Include CO2 reading only if the sensor has finished warming up.
		if CO2_ppm > 0 then
			data.CO2 = CO2_ppm
		end
	end

	if config.HAVE_BH1750 then
		-- XXX: Kludge: Do the I2C setup again (see comment in sensor_setup).
		local bh_sda, bh_scl = 2, 7
		i2c.setup(0, bh_sda, bh_scl, i2c.SLOW)

		-- Read illuminance from BH1750 sensor at default address 0x23.
		i2c.start(0)
		i2c.address(0, 0x23, i2c.TRANSMITTER)
		i2c.write(0, 0x10) -- Continuous high-res mode.
		i2c.stop(0)
		i2c.start(0)
		i2c.address(0, 0x23, i2c.RECEIVER)
		tmr.delay(200)
		local val = i2c.read(0, 2)
		i2c.stop(0)

		local lux = (val:byte(1) * 256 + val:byte(2)) * 1000 / 1200
		data.Ev = lux
	end

	-- Encode readings into JSON, terminate with LF.
	local json = sjson.encode(data)..string.char(10)

	-- Print encoded JSON for debugging.
	print(json)

	-- Send readings to remote server over the pre-established TLS connection.
	tls_send(json)
end

function wifi_connect(ssid, password)
	wifi.setmode(wifi.STATION)
	cfg = {}
	cfg.ssid = ssid
	cfg.pwd  = password
	cfg.save = false
	wifi.sta.config(cfg)
	wifi.sta.connect()
end

function wifi_wait4ip(callback)
	tmr.create():alarm(1000, tmr.ALARM_AUTO,
		function(t)
			if wifi.sta.getip() ~= nil then
				t:unregister()
				callback()
			end
		end)
end

function tls_connect()
	SRV:connect(config.SERVER_PORT, config.SERVER_ADDR)
end

function tls_init()
	-- Enable server certificate verification.
	tls.cert.verify(config.SERVER_CERT)

	-- Enable client authentication.
	-- TODO: tls.cert.auth(config.CLIENT_CERT, config.CLIENT_PRIVATE_KEY)

	-- Connect to the server.
	SRV = tls.createConnection()
	SRV:on("connection",
		function(sock, c)
			SRV_connected = true
			SRV_reconnecting = false
			print("[SERV] Connection to server established.")
		end)
	SRV:on("disconnection",
		function(sock, c)
			SRV_connected = false
			SRV_reconnecting = true
			print("[SERV] Connection to server lost, reconnecting...")

			-- Reconnect after 5 to 10 seconds.
			local fudge = node.random(0, 5000)
			tmr.create():alarm(5000 + fudge, tmr.ALARM_SINGLE,
				function(t)
					tls_connect()
				end)
		end)
	tls_connect()
end

function tls_send(data)
	-- Only send if the connection was established.
	if SRV_connected then
		SRV:send(data)
	elseif not SRV_reconnecting then
		tls_connect()
	end
end

function time_setup()
	local sync_success =
		function(sec, usec, serv, info)
			local t = rtctime.epoch2cal(sec)
			local correction

			if     info["offset_s"]  ~= nil then
				correction = string.format("Correction: %d s", info.offset_s)
			elseif info["offset_us"] ~= nil then
				correction = string.format("Correction: %d us", info.offset_us)
			else
				correction = "Correction: none"
			end

			print(string.format(
				"[SNTP] Date: %04d-%02d-%02d  Time: %02d:%02d:%02d  RTT: %d us  %s",
				t["year"], t["mon"], t["day"],
				t["hour"], t["min"], t["sec"],
				info.delay_us,
				correction))
		end

	local sync_failure =
		function(err_type, err_detail)
			local err
			if     err_type == 1 then
				err = "DNS lookup failed: " .. err_detail
			elseif err_type == 2 then
				err = "Memory allocation failure"
			elseif err_type == 3 then
				err = "UDP send failed"
			elseif err_type == 4 then
				err = "Timeout, no NTP response received"
			else
				err = "Unknown error"
			end
			print(string.format("[SNTP] Sync failed: %s.", err))
		end

	sntp.sync(config.NTP_ADDR, sync_success, sync_failure, 1)
end

function after_wifi_connects()
	local ip, nm, gw = wifi.sta.getip()
	print(string.format(
		"[WIFI] Connected.  My IP: %s  Netmask: %s  Gateway: %s",
		ip, nm, gw))

	-- Set-up NTP.
	time_setup()

	-- Establish a TLS connection to the server.
	tls_init()

	-- Read and report sensor data once per minute.
	cron.schedule("* * * * *", sensor_report)
end


sensor_setup()
wifi_connect(config.WIFI_SSID, config.WIFI_PASS)
wifi_wait4ip(after_wifi_connects)

