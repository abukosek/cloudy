package main

import (
	"bytes"
	"context"

	"github.com/oasisprotocol/oasis-core/go/common/cbor"
	"github.com/oasisprotocol/oasis-core/go/common/quantity"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/client"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/crypto/signature"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/crypto/signature/ed25519"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/accounts"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/modules/contracts"
	"github.com/oasisprotocol/oasis-sdk/client-sdk/go/types"
)

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

	tb := ct.Call(instanceID, req, []types.BaseUnits{}).
		SetFeeGas(350_000).
		SetFeeAmount(types.NewBaseUnits(*quantity.NewFromUint64(35_000_000), types.NativeDenomination)).
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
