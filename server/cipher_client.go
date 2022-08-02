package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"strings"

	"github.com/oasisprotocol/oasis-core/go/common/cbor"
	cmnGrpc "github.com/oasisprotocol/oasis-core/go/common/grpc"
	"github.com/oasisprotocol/oasis-core/go/common/quantity"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/crypto/signature"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/crypto/signature/ed25519"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/accounts"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/contracts"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Connect establishes a connection with the target rpc endpoint, omitting the
// chain context check.
func Connect(rpc string) (*grpc.ClientConn, error) {
	var dialOpts []grpc.DialOption
	switch strings.HasPrefix(rpc, "unix:") {
	case true:
		// No TLS needed for local nodes.
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	case false:
		// Configure TLS for non-local nodes.
		creds := credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
	}

	conn, err := cmnGrpc.Dial(rpc, dialOpts...)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// SignAndSubmitTx signs and submits the given request.
// TODO: Gas estimation is currently hardcoded.
func SignAndSubmitTx(ctx context.Context, rtc client.RuntimeClient, signer signature.Signer, req Request, instanceID contracts.InstanceID) (*Response, error) {
	ac := accounts.NewV1(rtc)
	ct := contracts.NewV1(rtc)

	// Get current nonce for the signer's account.
	sigSpec := types.NewSignatureAddressSpecEd25519(signer.Public().(ed25519.PublicKey))
	address := types.NewAddress(sigSpec)
	nonce, err := ac.Nonce(ctx, client.RoundLatest, address)
	if err != nil {
		return nil, err
	}

	// Educated guess for gas limit.
	gasLimit := uint64(350_000)

	// Add additional gas, if request is longer.
	reqLen := uint64(len(cbor.Marshal(req)))
	if reqLen > 130 {
		gasLimit += (reqLen - 130) * 2700
	}

	tb := ct.Call(instanceID, req, []types.BaseUnits{}).
		SetFeeGas(gasLimit).
		SetFeeAmount(types.NewBaseUnits(*quantity.NewFromUint64(gasLimit * 100), types.NativeDenomination)).
		AppendAuthSignature(sigSpec, nonce)

	if err := tb.AppendSign(ctx, signer); err != nil {
		return nil, err
	}

	var rawResult contracts.CallResult
	if err := tb.SubmitTx(ctx, &rawResult); err != nil {
		return nil, err
	}

	// TODO: Empty response is a simple string and cbor doesn't know how to decode it.
	if bytes.Compare(rawResult, EmptyResponseRaw) == 0 {
		return &Response{Empty: &EmptyResponse{}}, nil
	}

	// Otherwise, decode it into Response object.
	var result Response
	if err := cbor.Unmarshal(rawResult, &result); err != nil {
		return nil, err
	}

	return &result, nil
}
