#!/usr/bin/env gmake
#
# Makefile for the Go server.
#

GO ?= go

all: server.go cipher_client.go
	@echo "*** Building Go server..."
	@$(GO) get
	@$(GO) build

clean:
	@echo "*** Cleaning up..."
	@-rm -f server

key:
	@test -f server.key || (echo "*** Generating server private key..."; openssl ecparam -genkey -name secp256r1 -out server.key)
	@test -f server.crt || (echo "*** Generating server certificate..."; openssl req -new -x509 -sha256 -key server.key -out server.crt -days 3650 -batch)

clean-key:
	@echo "*** Cleaning up server key and certificate..."
	@-rm -f server.crt server.key

fmt:
	@echo "*** Formatting source files..."
	@$(GO) fmt server.go

